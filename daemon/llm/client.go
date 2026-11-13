package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"swcode/daemon/protocol"
)

const (
	chatPath       = "/v1/chat/completions"
	callIDPrefix   = "call_"
	callIDBytes    = 16
	maxStreamLine  = 16 << 20
	missingPicture = "[A pasted picture that is no longer there.]"
	// How llm-server says the prompt does not fit in the slot's context, and how OpenAI's API says it.
	contextFullType = "exceed_context_size_error"
	contextFullCode = "context_length_exceeded"
)

var contextFullMessage = regexp.MustCompile(`(?i)exceeds? the (available context size|context window)`)

// retriedStatuses are the answers of an llm-server out of reach for a moment: 502 from the proxy before it (nginx)
// while it restarts, or when the proxy sent the request on a kept-alive connection llm-server had just closed; 503
// while it loads its model. Nothing of the reply has come, so the request is sent again.
var retriedStatuses = map[int]bool{
	http.StatusBadGateway:         true,
	http.StatusServiceUnavailable: true,
}

// retryWaits are the waits before each new try: about 15 seconds in all before the turn fails.
var retryWaits = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}

// llamaFields are llama.cpp's own fields of a request, beyond the Chat Completions API: OpenAI's API and others
// refuse them ("Unknown parameter: 'cache_prompt'").
var llamaFields = []string{"cache_prompt", "id_slot", "chat_template_kwargs", "reasoning_content"}

// templateKwargs is what each thinking mode asks the chat template, through llm-server's chat_template_kwargs:
// nothing for the model's own default, a level the Qwen 3.8 and gpt-oss templates know, or no thinking at all,
// which Qwen templates take as instruct mode. A template that knows none of these ignores them.
var templateKwargs = map[protocol.ThinkingMode]map[string]any{
	protocol.ThinkingModeFull:   nil,
	protocol.ThinkingModeLow:    {"reasoning_effort": "low"},
	protocol.ThinkingModeMedium: {"reasoning_effort": "medium"},
	protocol.ThinkingModeOff:    {"enable_thinking": false},
}

// Options is where llm-server is, the model to name when a request names none, and its key.
type Options struct {
	URL    string
	Model  string
	APIKey string
	HTTP   *http.Client
}

// BaseURL is the server's address without a trailing slash, so paths can be appended to it.
func BaseURL(url string) string {
	return strings.TrimRight(url, "/")
}

func (o Options) client() *http.Client {
	if o.HTTP != nil {
		return o.HTTP
	}
	return http.DefaultClient
}

func (o Options) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, BaseURL(o.URL)+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("content-type", "application/json")
	if o.APIKey != "" {
		request.Header.Set("authorization", protocol.BearerPrefix+o.APIKey)
	}
	return request, nil
}

// post sends the body to the path, and again after a wait while llm-server is out of reach for a moment.
func (o Options) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	for _, wait := range retryWaits {
		response, err := o.postOnce(ctx, path, body)
		if err != nil || !retriedStatuses[response.StatusCode] {
			return response, err
		}
		response.Body.Close()
		if err := pause(ctx, wait); err != nil {
			return nil, err
		}
	}
	return o.postOnce(ctx, path, body)
}

func (o Options) postOnce(ctx context.Context, path string, body []byte) (*http.Response, error) {
	request, err := o.newRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	response, err := o.client().Do(request)
	if err != nil {
		// Stopping the turn ends the request: that is not llm-server's fault.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ModelError{Message: fmt.Sprintf("llm-server at %s does not answer.", o.URL)}
	}
	return response, nil
}

// pause waits, unless the turn stops first.
func pause(ctx context.Context, wait time.Duration) error {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// The API's shapes, in its snake_case.
type wireDelta struct {
	Content          *string `json:"content"`
	ReasoningContent *string `json:"reasoning_content"`
	ToolCalls        []struct {
		Function *struct {
			Arguments *string `json:"arguments"`
			Name      *string `json:"name"`
		} `json:"function"`
		ID    *string `json:"id"`
		Index int     `json:"index"`
	} `json:"tool_calls"`
}

type wireChunk struct {
	Choices []struct {
		Delta *wireDelta `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage *struct {
		CompletionTokens int `json:"completion_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		// OpenAI's API says here how much of the prompt it had kept.
		PromptTokensDetails *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	// llama.cpp's own account of the request: the prompt's tokens it had kept (cache_n) and those it processed
	// (prompt_n), the tokens it wrote, and the time each took.
	Timings *struct {
		CacheN      int     `json:"cache_n"`
		PromptN     int     `json:"prompt_n"`
		PromptMs    float64 `json:"prompt_ms"`
		PredictedN  int     `json:"predicted_n"`
		PredictedMs float64 `json:"predicted_ms"`
	} `json:"timings"`
}

func text(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (w wireChunk) chunk() StreamChunk {
	chunk := StreamChunk{}
	if w.Usage != nil {
		chunk.Usage = &protocol.Usage{CompletionTokens: w.Usage.CompletionTokens, PromptTokens: w.Usage.PromptTokens}
		if details := w.Usage.PromptTokensDetails; details != nil {
			chunk.Usage.CachedTokens = details.CachedTokens
		}
	}
	if w.Timings != nil {
		chunk.Timings = &Timings{
			CachedTokens: w.Timings.CacheN, CompletionTokens: w.Timings.PredictedN, PredictedMs: int(math.Round(w.Timings.PredictedMs)),
			PromptMs: int(math.Round(w.Timings.PromptMs)), PromptTokens: w.Timings.PromptN + w.Timings.CacheN,
		}
	}
	if len(w.Choices) == 0 || w.Choices[0].Delta == nil {
		return chunk
	}
	delta := w.Choices[0].Delta
	chunk.Text, chunk.Reasoning = text(delta.Content), text(delta.ReasoningContent)
	for _, call := range delta.ToolCalls {
		next := ToolCallDelta{ID: text(call.ID), Index: call.Index}
		if call.Function != nil {
			next.Arguments, next.Name = text(call.Function.Arguments), text(call.Function.Name)
		}
		chunk.ToolCalls = append(chunk.ToolCalls, next)
	}
	return chunk
}

// errorBody is how llm-server explains a refusal: a message, or an object with a message, a type and a code.
type errorBody struct {
	Error json.RawMessage `json:"error"`
}

// errorDetail is a refusal's message, and what kind it is: llama.cpp says it in its type, OpenAI's API in its code.
type errorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func (b errorBody) detail() errorDetail {
	var text string
	if json.Unmarshal(b.Error, &text) == nil {
		return errorDetail{Message: text}
	}
	var detail errorDetail
	_ = json.Unmarshal(b.Error, &detail)
	return detail
}

func (b errorBody) message() string {
	return b.detail().Message
}

// contextFull tells a refusal for a prompt too long for the model's context.
func (d errorDetail) contextFull() bool {
	return d.Type == contextFullType || d.Code == contextFullCode || contextFullMessage.MatchString(d.Message)
}

// refusalOf is the error for a refused request: a full context is told apart, the turn can compact and go on.
func refusalOf(status int, body []byte, statusText string) *ModelError {
	var parsed errorBody
	_ = json.Unmarshal(body, &parsed)
	detail := parsed.detail()
	return &ModelError{
		ContextFull: detail.contextFull(),
		Message:     fmt.Sprintf("llm-server answered %d: %s", status, firstNonEmpty(detail.Message, statusText)),
	}
}

// pictureURL is a picture inline, as a data URL; false once it is gone from the disk.
func pictureURL(picture Picture) (string, bool) {
	data, err := os.ReadFile(picture.Path)
	if err != nil {
		return "", false
	}
	return "data:" + picture.Type + ";base64," + base64.StdEncoding.EncodeToString(data), true
}

// pictureContent is a picture as the Chat Completions API takes it inline; one gone from the disk is said to be gone.
func pictureContent(picture Picture) map[string]any {
	url, found := pictureURL(picture)
	if !found {
		return map[string]any{"text": missingPicture, "type": "text"}
	}
	return map[string]any{"image_url": map[string]any{"url": url}, "type": "image_url"}
}

// wireContent is a message's content: its text, then its pictures, when it has some.
func wireContent(message Message) any {
	if message.Role != RoleUser || len(message.Pictures) == 0 {
		return message.Content
	}
	content := []map[string]any{{"text": message.Content, "type": "text"}}
	for _, picture := range message.Pictures {
		content = append(content, pictureContent(picture))
	}
	return content
}

func wireMessage(message Message, plain bool) map[string]any {
	wire := map[string]any{"content": wireContent(message), "role": message.Role}
	if message.Role == RoleAssistant && len(message.ToolCalls) > 0 {
		calls := []map[string]any{}
		for _, call := range message.ToolCalls {
			calls = append(calls, map[string]any{
				"function": map[string]any{"arguments": call.Arguments, "name": call.Name},
				"id":       call.ID,
				"type":     "function",
			})
		}
		wire["tool_calls"] = calls
	}
	if message.Role == RoleTool {
		wire["tool_call_id"] = message.ToolCallID
	}
	if !plain && message.Role == RoleAssistant && message.Reasoning != "" {
		wire["reasoning_content"] = message.Reasoning
	}
	return wire
}

// Client is llm-server's Chat Completions API, streamed, with its prompt cache on so a long conversation stays
// fast.
type Client struct {
	options Options
	// plain: the server refused llama.cpp's own fields, as OpenAI's API does; its requests go without them.
	plain atomic.Bool
}

func NewClient(options Options) *Client {
	return &Client{options: options}
}

// bodyOf is the request as the API takes it: with llama.cpp's own fields, unless plain.
func bodyOf(request Request, model string, plain bool) map[string]any {
	messages := []map[string]any{}
	for _, message := range request.Messages {
		messages = append(messages, wireMessage(message, plain))
	}
	body := map[string]any{
		"messages":       messages,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	// A summary request has no tools: some servers refuse an empty list.
	if len(request.Tools) > 0 {
		tools := []map[string]any{}
		for _, tool := range request.Tools {
			tools = append(tools, map[string]any{"function": tool, "type": "function"})
		}
		body["tools"] = tools
	}
	if model := firstNonEmpty(request.Target.Model, model); model != "" {
		body["model"] = model
	}
	if plain {
		return body
	}
	// Reuses the KV cache of the conversation's earlier turns: only what is new gets processed.
	body["cache_prompt"] = true
	// Each agent keeps to its slot, where its conversation is still cached.
	if request.Target.Slot != NoSlot {
		body["id_slot"] = request.Target.Slot
	}
	if kwargs := templateKwargs[request.Target.Thinking]; kwargs != nil {
		body["chat_template_kwargs"] = kwargs
	}
	return body
}

// namesLlamaField tells a refusal that names one of llama.cpp's own fields: the server does not know it.
func namesLlamaField(refusal []byte) bool {
	return slices.ContainsFunc(llamaFields, func(field string) bool { return bytes.Contains(refusal, []byte(field)) })
}

// send posts the request once, with llama.cpp's own fields unless plain.
func (c *Client) send(ctx context.Context, request Request, plain bool) (*http.Response, error) {
	body, err := json.Marshal(bodyOf(request, c.options.Model, plain))
	if err != nil {
		return nil, err
	}
	return c.options.post(ctx, chatPath, body)
}

// post sends the request, and once more without llama.cpp's own fields when the server refuses one of them, as
// OpenAI's API does: the client remembers it, and the server's next requests go without them at once.
func (c *Client) post(ctx context.Context, request Request) (*http.Response, error) {
	plain := c.plain.Load()
	response, err := c.send(ctx, request, plain)
	if err != nil || plain || response.StatusCode != http.StatusBadRequest {
		return response, err
	}
	refusal, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if !namesLlamaField(refusal) {
		response.Body = io.NopCloser(bytes.NewReader(refusal))
		return response, nil
	}
	c.plain.Store(true)
	return c.send(ctx, request, true)
}

func (c *Client) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	response, err := c.post(ctx, request)
	if err != nil {
		return Reply{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		refusal, _ := io.ReadAll(response.Body)
		return Reply{}, refusalOf(response.StatusCode, refusal, http.StatusText(response.StatusCode))
	}
	return readReply(ctx, response.Body, onPart)
}

// readReply reads the stream into the reply; an error llm-server reports mid-stream ends it.
func readReply(ctx context.Context, body io.Reader, onPart func(Part)) (Reply, error) {
	builder := &ReplyBuilder{}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	for scanner.Scan() {
		if err := readLine(strings.TrimSuffix(scanner.Text(), "\r"), builder, onPart); err != nil {
			return Reply{}, err
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, &ModelError{Message: "llm-server broke off its answer: " + err.Error()}
	}
	return builder.Build(newCallID), nil
}

func readLine(line string, builder *ReplyBuilder, onPart func(Part)) error {
	sse := ReadSSELine(line)
	if sse.Kind == SSEOther || sse.Data == DoneMarker {
		return nil
	}
	if sse.Kind == SSEError {
		var body errorBody
		_ = json.Unmarshal([]byte(sse.Data), &body)
		return &ModelError{Message: firstNonEmpty(body.message(), sse.Data)}
	}
	var chunk wireChunk
	if json.Unmarshal([]byte(sse.Data), &chunk) != nil {
		return nil
	}
	if chunk.Error != nil {
		return &ModelError{Message: chunk.Error.Message}
	}
	for _, part := range builder.Add(chunk.chunk()) {
		onPart(part)
	}
	return nil
}

func newCallID() string {
	id := make([]byte, callIDBytes)
	_, _ = rand.Read(id)
	return callIDPrefix + hex.EncodeToString(id)
}

// IsContextFull tells a refusal for a prompt too long for the slot.
func IsContextFull(err error) bool {
	var model *ModelError
	return errors.As(err, &model) && model.ContextFull
}

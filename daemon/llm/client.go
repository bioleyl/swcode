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
	"net/http"
	"os"
	"regexp"
	"strings"

	"swcode/daemon/protocol"
)

const (
	chatPath       = "/v1/chat/completions"
	callIDPrefix   = "call_"
	callIDBytes    = 16
	maxStreamLine  = 16 << 20
	missingPicture = "[A pasted picture that is no longer there.]"
	// How llm-server says the prompt does not fit in the slot's context.
	contextFullType = "exceed_context_size_error"
)

var contextFullMessage = regexp.MustCompile(`(?i)exceeds? the available context size`)

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
	} `json:"usage"`
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

// errorBody is how llm-server explains a refusal: a message, or an object with a message and a type.
type errorBody struct {
	Error json.RawMessage `json:"error"`
}

func (b errorBody) message() (string, string) {
	var text string
	if json.Unmarshal(b.Error, &text) == nil {
		return text, ""
	}
	var detail struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	}
	if json.Unmarshal(b.Error, &detail) == nil {
		return detail.Message, detail.Type
	}
	return "", ""
}

// refusalOf is the error for a refused request: a full context is told apart, the turn can compact and go on.
func refusalOf(status int, body []byte, statusText string) *ModelError {
	var parsed errorBody
	_ = json.Unmarshal(body, &parsed)
	detail, kind := parsed.message()
	detail = firstNonEmpty(detail, statusText)
	return &ModelError{
		ContextFull: kind == contextFullType || contextFullMessage.MatchString(detail),
		Message:     fmt.Sprintf("llm-server answered %d: %s", status, detail),
	}
}

// pictureContent is a picture as the Chat Completions API takes it inline; one gone from the disk is said to be gone.
func pictureContent(picture Picture) map[string]any {
	data, err := os.ReadFile(picture.Path)
	if err != nil {
		return map[string]any{"text": missingPicture, "type": "text"}
	}
	url := "data:" + picture.Type + ";base64," + base64.StdEncoding.EncodeToString(data)
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

func wireMessage(message Message) map[string]any {
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
	if message.Role == RoleAssistant && message.Reasoning != "" {
		wire["reasoning_content"] = message.Reasoning
	}
	return wire
}

// Client is llm-server's Chat Completions API, streamed, with its prompt cache on so a long conversation stays
// fast.
type Client struct {
	options Options
}

func NewClient(options Options) *Client {
	return &Client{options: options}
}

func (c *Client) bodyOf(request Request) map[string]any {
	messages := []map[string]any{}
	for _, message := range request.Messages {
		messages = append(messages, wireMessage(message))
	}
	body := map[string]any{
		// Reuses the KV cache of the conversation's earlier turns: only what is new gets processed.
		"cache_prompt":   true,
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
	if model := firstNonEmpty(request.Target.Model, c.options.Model); model != "" {
		body["model"] = model
	}
	// Each agent keeps to its slot, where its conversation is still cached.
	if request.Target.Slot != NoSlot {
		body["id_slot"] = request.Target.Slot
	}
	if kwargs := templateKwargs[request.Target.Thinking]; kwargs != nil {
		body["chat_template_kwargs"] = kwargs
	}
	return body
}

func (c *Client) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	body, err := json.Marshal(c.bodyOf(request))
	if err != nil {
		return Reply{}, err
	}
	httpRequest, err := c.options.newRequest(ctx, http.MethodPost, chatPath, bytes.NewReader(body))
	if err != nil {
		return Reply{}, err
	}
	response, err := c.options.client().Do(httpRequest)
	if err != nil {
		// Stopping the turn ends the request: that is not llm-server's fault.
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, &ModelError{Message: fmt.Sprintf("llm-server at %s does not answer.", c.options.URL)}
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
		message, _ := body.message()
		return &ModelError{Message: firstNonEmpty(message, sse.Data)}
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

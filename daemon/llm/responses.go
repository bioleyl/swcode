package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"swcode/daemon/protocol"
)

const (
	responsesPath = "/v1/responses"
	// openAIHost is OpenAI's own server: its newer models take tools and thinking together only through the
	// Responses API, which also gives their reasoning back, encrypted, to go on from at the next step.
	openAIHost = "api.openai.com"
	// encryptedReasoning asks for the reasoning items' encrypted content: nothing is stored on OpenAI's side.
	encryptedReasoning = "reasoning.encrypted_content"
	// summaryAuto asks for a summary of the reasoning, which streams into the conversation as the thinking.
	summaryAuto = "auto"
	// summarySeparator is between two parts of a reasoning summary.
	summarySeparator = "\n\n"
)

// Responses API stream events, by their type.
const (
	eventTextDelta      = "response.output_text.delta"
	eventSummaryDelta   = "response.reasoning_summary_text.delta"
	eventSummaryPart    = "response.reasoning_summary_part.added"
	eventItemAdded      = "response.output_item.added"
	eventArgumentsDelta = "response.function_call_arguments.delta"
	eventCompleted      = "response.completed"
	eventIncomplete     = "response.incomplete"
	eventFailed         = "response.failed"
	eventError          = "error"
	itemFunctionCall    = "function_call"
)

// responsesEfforts is each thinking mode as the Responses API's reasoning effort: nothing for the model's own.
var responsesEfforts = map[protocol.ThinkingMode]string{
	protocol.ThinkingModeFull:   "",
	protocol.ThinkingModeLow:    "low",
	protocol.ThinkingModeMedium: "medium",
	protocol.ThinkingModeOff:    "none",
}

// reasoningRefusals are the words of a refusal about the reasoning settings: a model that does not think, does not
// know the effort asked, or an organization not allowed reasoning summaries.
var reasoningRefusals = []string{"'reasoning", "reasoning.effort", "reasoning.summary", "reasoning summar", encryptedReasoning}

// reasoningRule is what a model's refusals taught about the reasoning settings, each rule sending fewer of them.
type reasoningRule int

const (
	// reasoningWhole: the effort the thinking mode asks, its summary, and its encrypted content.
	reasoningWhole reasoningRule = iota
	// reasoningUnsummarized: no summary, which an organization may not be allowed.
	reasoningUnsummarized
	// reasoningNone: no reasoning settings: the model does not think, or not as asked.
	reasoningNone
)

// UsesResponses tells a server whose models are asked through OpenAI's Responses API: OpenAI's own.
func UsesResponses(address string) bool {
	parsed, err := url.Parse(address)
	return err == nil && parsed.Hostname() == openAIHost
}

// ResponsesClient is OpenAI's Responses API, streamed and stateless: each request sends the whole conversation,
// each reply's items (its encrypted reasoning with them) going back as they came.
type ResponsesClient struct {
	options Options
	mu      sync.Mutex
	rules   map[string]reasoningRule
}

func NewResponsesClient(options Options) *ResponsesClient {
	return &ResponsesClient{options: options, rules: map[string]reasoningRule{}}
}

func (c *ResponsesClient) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	response, err := c.post(ctx, request, firstNonEmpty(request.Target.Model, c.options.Model))
	if err != nil {
		return Reply{}, err
	}
	defer response.Body.Close()
	if response.StatusCode >= http.StatusBadRequest {
		refusal, _ := io.ReadAll(response.Body)
		return Reply{}, refusalOf(response.StatusCode, refusal, http.StatusText(response.StatusCode))
	}
	return readResponses(ctx, response.Body, onPart)
}

// post sends the request, and again with fewer reasoning settings while the model refuses them: the client
// remembers it, the model's next requests go without them at once.
func (c *ResponsesClient) post(ctx context.Context, request Request, model string) (*http.Response, error) {
	for {
		rule := c.ruleOf(model)
		response, err := c.send(ctx, responsesBody(request, model, rule))
		if err != nil || response.StatusCode != http.StatusBadRequest {
			return response, err
		}
		refusal, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if !c.learn(model, rule, refusal) {
			response.Body = io.NopCloser(bytes.NewReader(refusal))
			return response, nil
		}
	}
}

func (c *ResponsesClient) ruleOf(model string) reasoningRule {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rules[model]
}

// learn reads a refusal of a request sent under a rule, and tells whether the next try is sent otherwise: the rules
// only go forward, so the tries end.
func (c *ResponsesClient) learn(model string, sent reasoningRule, refusal []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	refusesReasoning := slices.ContainsFunc(reasoningRefusals, func(words string) bool { return bytes.Contains(refusal, []byte(words)) })
	if refusesReasoning && sent < reasoningNone {
		c.rules[model] = max(c.rules[model], sent+1)
	}
	return c.rules[model] != sent
}

func (c *ResponsesClient) send(ctx context.Context, body map[string]any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := c.options.newRequest(ctx, http.MethodPost, responsesPath, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	response, err := c.options.client().Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &ModelError{Message: fmt.Sprintf("llm-server at %s does not answer.", c.options.URL)}
	}
	return response, nil
}

// responsesBody is the request as the Responses API takes it, with the reasoning settings the model's rule allows.
// Nothing is stored on OpenAI's side.
func responsesBody(request Request, model string, rule reasoningRule) map[string]any {
	body := map[string]any{"input": responsesInput(request.Messages), "store": false, "stream": true}
	if model != "" {
		body["model"] = model
	}
	if len(request.Tools) > 0 {
		body["tools"] = responsesTools(request.Tools)
	}
	if rule == reasoningNone {
		return body
	}
	body["include"] = []string{encryptedReasoning}
	if reasoning := reasoningOf(request.Target.Thinking, rule); len(reasoning) > 0 {
		body["reasoning"] = reasoning
	}
	return body
}

func reasoningOf(thinking protocol.ThinkingMode, rule reasoningRule) map[string]any {
	reasoning := map[string]any{}
	if effort := responsesEfforts[thinking]; effort != "" {
		reasoning["effort"] = effort
	}
	if rule == reasoningWhole {
		reasoning["summary"] = summaryAuto
	}
	return reasoning
}

// responsesTools is the tools as the Responses API takes them: not strict, which would want every property
// required.
func responsesTools(definitions []ToolDefinition) []map[string]any {
	tools := []map[string]any{}
	for _, tool := range definitions {
		tools = append(tools, map[string]any{
			"description": tool.Description, "name": tool.Name, "parameters": tool.Parameters, "strict": false,
			"type": "function",
		})
	}
	return tools
}

// inputOf turns each role's message into the Responses API's input items.
var inputOf = map[Role]func(Message) []any{
	RoleSystem: func(message Message) []any {
		return []any{map[string]any{"content": message.Content, "role": RoleSystem}}
	},
	RoleUser:      func(message Message) []any { return []any{userInput(message)} },
	RoleAssistant: assistantInput,
	RoleTool: func(message Message) []any {
		return []any{map[string]any{"call_id": message.ToolCallID, "output": message.Content, "type": "function_call_output"}}
	},
}

func responsesInput(messages []Message) []any {
	input := []any{}
	for _, message := range messages {
		if items, known := inputOf[message.Role]; known {
			input = append(input, items(message)...)
		}
	}
	return input
}

// userInput is a user message: its text, then its pictures, when it has some.
func userInput(message Message) map[string]any {
	if len(message.Pictures) == 0 {
		return map[string]any{"content": message.Content, "role": RoleUser}
	}
	content := []map[string]any{{"text": message.Content, "type": "input_text"}}
	for _, picture := range message.Pictures {
		url, found := pictureURL(picture)
		if !found {
			content = append(content, map[string]any{"text": missingPicture, "type": "input_text"})
			continue
		}
		content = append(content, map[string]any{"image_url": url, "type": "input_image"})
	}
	return map[string]any{"content": content, "role": RoleUser}
}

// assistantInput is a reply: its items as the Responses API wrote them, else (a reply of another server's, before
// the preset changed) its text and calls, its reasoning left out.
func assistantInput(message Message) []any {
	var items []json.RawMessage
	if message.Items != "" && json.Unmarshal([]byte(message.Items), &items) == nil {
		return toAny(items)
	}
	input := []any{}
	if message.Content != "" {
		input = append(input, map[string]any{"content": message.Content, "role": RoleAssistant})
	}
	for _, call := range message.ToolCalls {
		input = append(input, map[string]any{"arguments": call.Arguments, "call_id": call.ID, "name": call.Name, "type": itemFunctionCall})
	}
	return input
}

func toAny(items []json.RawMessage) []any {
	list := make([]any, len(items))
	for index, item := range items {
		list[index] = item
	}
	return list
}

// responsesEvent is one event of the stream, the fields of all its types together.
type responsesEvent struct {
	Type         string `json:"type"`
	Delta        string `json:"delta"`
	OutputIndex  int    `json:"output_index"`
	SummaryIndex int    `json:"summary_index"`
	Item         *struct {
		Type   string `json:"type"`
		CallID string `json:"call_id"`
		Name   string `json:"name"`
	} `json:"item"`
	Response *struct {
		Output json.RawMessage `json:"output"`
		Usage  *struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Error *errorDetail `json:"error"`
	} `json:"response"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// failure is the error a failed response or an error event reports; a full context is told apart.
func (e responsesEvent) failure() error {
	if e.Type != eventFailed && e.Type != eventError {
		return nil
	}
	detail := errorDetail{Code: e.Code, Message: e.Message}
	if e.Response != nil && e.Response.Error != nil {
		detail = *e.Response.Error
	}
	return &ModelError{ContextFull: detail.contextFull(), Message: firstNonEmpty(detail.Message, "llm-server broke off its answer.")}
}

// finished tells the last event of a response, which holds its items and its tokens.
func (e responsesEvent) finished() bool {
	return (e.Type == eventCompleted || e.Type == eventIncomplete) && e.Response != nil
}

// chunkOf is what each event of the stream adds to the reply.
var chunkOf = map[string]func(responsesEvent) StreamChunk{
	eventTextDelta:    func(event responsesEvent) StreamChunk { return StreamChunk{Text: event.Delta} },
	eventSummaryDelta: func(event responsesEvent) StreamChunk { return StreamChunk{Reasoning: event.Delta} },
	eventSummaryPart: func(event responsesEvent) StreamChunk {
		if event.SummaryIndex == 0 {
			return StreamChunk{}
		}
		return StreamChunk{Reasoning: summarySeparator}
	},
	eventItemAdded: func(event responsesEvent) StreamChunk {
		if event.Item == nil || event.Item.Type != itemFunctionCall {
			return StreamChunk{}
		}
		return StreamChunk{ToolCalls: []ToolCallDelta{{ID: event.Item.CallID, Index: event.OutputIndex, Name: event.Item.Name}}}
	},
	eventArgumentsDelta: func(event responsesEvent) StreamChunk {
		return StreamChunk{ToolCalls: []ToolCallDelta{{Arguments: event.Delta, Index: event.OutputIndex}}}
	},
}

// readResponses reads the stream into the reply, with its items as the last event gives them.
func readResponses(ctx context.Context, body io.Reader, onPart func(Part)) (Reply, error) {
	builder := &ReplyBuilder{}
	items := ""
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64<<10), maxStreamLine)
	for scanner.Scan() {
		sse := ReadSSELine(strings.TrimSuffix(scanner.Text(), "\r"))
		var event responsesEvent
		if sse.Kind != SSEData || json.Unmarshal([]byte(sse.Data), &event) != nil {
			continue
		}
		if err := event.failure(); err != nil {
			return Reply{}, err
		}
		if event.finished() {
			items = finish(builder, event)
			continue
		}
		if chunk, known := chunkOf[event.Type]; known {
			for _, part := range builder.Add(chunk(event)) {
				onPart(part)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, &ModelError{Message: "llm-server broke off its answer: " + err.Error()}
	}
	reply := builder.Build(newCallID)
	reply.Items = items
	return reply, nil
}

// finish takes the last event's tokens into the reply, and gives its items.
func finish(builder *ReplyBuilder, event responsesEvent) string {
	if usage := event.Response.Usage; usage != nil {
		builder.Add(StreamChunk{Usage: &protocol.Usage{CompletionTokens: usage.OutputTokens, PromptTokens: usage.InputTokens}})
	}
	if len(event.Response.Output) == 0 {
		return ""
	}
	return string(event.Response.Output)
}

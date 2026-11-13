// Package fakellama is an llm-server stand-in for the tests: it streams the replies a test scripts, and records
// what the agent sent it.
package fakellama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

const (
	noMoreReplies = "The fake model has no more replies."
	noModel       = "model name is missing from the request"
	// unknownParameter is how OpenAI's API refuses a field it does not know.
	unknownParameter = "Unknown parameter: '%s'."
	// EncryptedReasoning is the encrypted content of the reasoning items the fake writes through the Responses API.
	EncryptedReasoning = "fake-encrypted-reasoning"
	completionTokens   = 40
	// charsPerToken: its token count is four characters a token of the messages, close enough for tests of the
	// context budget.
	charsPerToken = 4
)

// ToolCall is a call the fake model writes: its arguments are an object sent as its JSON, or the text itself.
type ToolCall struct {
	Name      string
	Arguments any
}

// Reply is one reply of the fake model: some text, some tool calls, or an error status instead.
type Reply struct {
	Text      string
	ToolCalls []ToolCall
	FailWith  int
	// ErrorType is the error's type, with FailWith: llm-server's own, such as exceed_context_size_error.
	ErrorType string
	// Reasoning is what the model thinks before it answers, streamed apart from the answer.
	Reasoning string
	// Delay is how long the model thinks before it answers, to see work in progress.
	Delay time.Duration
	// ChunkDelay is a pause between the chunks, to see the reply stream in.
	ChunkDelay time.Duration
}

// RouterModel is a model a router serves, with its own settings.
type RouterModel struct {
	ID            string
	Loaded        bool
	ContextTokens int
	Slots         int
	// Vision: the router lists the model as taking pictures, loaded or not, as with a vision projector (--mmproj).
	Vision bool
}

type Options struct {
	// Replies are the replies, in order: each chat completion takes the next one.
	Replies []Reply
	// Scripts are replies for the conversations whose first user message holds the key (a subagent's task), or
	// whose last one does (what swcode asks at the end of a conversation, a summary say), each in order: requests
	// that run at the same time (subagents) cannot share one script.
	Scripts       map[string][]Reply
	Models        []string
	ContextTokens int
	// Slots is what /props reports; 0, it does not say, like an older llm-server.
	Slots int
	// Vision: /props says the model sees pictures, as with a vision projector loaded.
	Vision bool
	// Router answers as a router serving these models: each has its own /props, and requests must name one.
	Router []RouterModel
	// Refuses is fields the server does not know, as OpenAI's API refuses llama.cpp's: a request that has one,
	// itself or in a message, gets a 400 that names it.
	Refuses []string
}

// Server is a running fake llm-server.
type Server struct {
	// URL is where it answers, without /v1, as LLAMA_URL takes it.
	URL        string
	options    Options
	server     *httptest.Server
	mu         sync.Mutex
	replies    []Reply
	scripts    map[string][]Reply
	requests   []map[string]any
	propsAsked []string
}

// Start starts a fake llm-server; Stop ends it.
func Start(options Options) *Server {
	scripts := map[string][]Reply{}
	for key, replies := range options.Scripts {
		scripts[key] = append([]Reply{}, replies...)
	}
	fake := &Server{options: options, replies: append([]Reply{}, options.Replies...), scripts: scripts}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.serve))
	fake.URL = fake.server.URL
	return fake
}

func (s *Server) Stop() { s.server.Close() }

// Requests is the body of every chat completion request, in order.
func (s *Server) Requests() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any{}, s.requests...)
}

// PropsAsked is the model each /props request named, in order; empty for none.
func (s *Server) PropsAsked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.propsAsked...)
}

func answer(response http.ResponseWriter, status int, body any) {
	response.Header().Set("content-type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(body)
}

func (s *Server) serve(response http.ResponseWriter, request *http.Request) {
	data, _ := io.ReadAll(request.Body)
	body := map[string]any{}
	_ = json.Unmarshal(data, &body)
	path := request.URL.Path
	switch {
	case path == "/v1/models":
		answer(response, http.StatusOK, s.models())
	case path == "/props":
		s.mu.Lock()
		s.propsAsked = append(s.propsAsked, request.URL.Query().Get("model"))
		s.mu.Unlock()
		answer(response, http.StatusOK, s.props(request.URL.Query().Get("model")))
	case path == "/v1/chat/completions":
		s.complete(request.Context(), response, body, stream)
	case path == "/v1/responses":
		s.complete(request.Context(), response, body, streamResponses)
	default:
		answer(response, http.StatusNotFound, map[string]any{"error": map[string]any{"message": "Not found"}})
	}
}

func (s *Server) models() any {
	data := []map[string]any{}
	if s.options.Router == nil {
		for _, id := range s.options.Models {
			data = append(data, map[string]any{"id": id, "object": "model"})
		}
		return map[string]any{"data": data}
	}
	for _, model := range s.options.Router {
		status := "unloaded"
		if model.Loaded {
			status = "loaded"
		}
		inputs := []string{"text"}
		if model.Vision {
			inputs = append(inputs, "image")
		}
		data = append(data, map[string]any{
			"architecture": map[string]any{"input_modalities": inputs, "output_modalities": []string{"text"}},
			"id":           model.ID, "object": "model", "status": map[string]any{"value": status},
		})
	}
	return map[string]any{"data": data}
}

// props is a router's: the asked model's settings, or the router's own, which have no context.
func (s *Server) props(model string) any {
	if s.options.Router == nil {
		props := map[string]any{
			"default_generation_settings": map[string]any{"n_ctx": s.options.ContextTokens},
			"modalities":                  map[string]any{"vision": s.options.Vision},
		}
		if s.options.Slots > 0 {
			props["total_slots"] = s.options.Slots
		}
		return props
	}
	for _, served := range s.options.Router {
		if served.ID == model {
			return map[string]any{"default_generation_settings": map[string]any{"n_ctx": served.ContextTokens}, "total_slots": served.Slots}
		}
	}
	return map[string]any{"default_generation_settings": map[string]any{"n_ctx": 0}, "role": "router"}
}

// complete answers a request with the next reply, streamed as its API does.
func (s *Server) complete(ctx context.Context, response http.ResponseWriter, body map[string]any, write func(http.ResponseWriter, Reply, int)) {
	s.mu.Lock()
	s.requests = append(s.requests, body)
	s.mu.Unlock()
	if _, named := body["model"].(string); s.options.Router != nil && !named {
		answer(response, http.StatusBadRequest, map[string]any{"error": map[string]any{"message": noModel, "type": "invalid_request_error"}})
		return
	}
	if field := s.refused(body); field != "" {
		answer(response, http.StatusBadRequest, map[string]any{"error": map[string]any{
			"message": fmt.Sprintf(unknownParameter, field), "param": field, "type": "invalid_request_error",
		}})
		return
	}
	reply := s.next(body)
	// A request given up (a stopped turn) ends the thinking: the server closes at once after its test.
	select {
	case <-time.After(reply.Delay):
	case <-ctx.Done():
		return
	}
	if reply.FailWith != 0 {
		message := reply.Text
		if message == "" {
			message = "Refused by the fake server"
		}
		answer(response, reply.FailWith, map[string]any{"error": map[string]any{"message": message, "type": reply.ErrorType}})
		return
	}
	write(response, reply, promptTokensOf(body))
}

// refused is the first field of the request the server does not know, itself or in one of its messages.
func (s *Server) refused(body map[string]any) string {
	messages, _ := body["messages"].([]any)
	for _, field := range s.options.Refuses {
		if _, found := body[field]; found {
			return field
		}
		for _, message := range messages {
			if fields, _ := message.(map[string]any); fields[field] != nil {
				return field
			}
		}
	}
	return ""
}

func (s *Server) next(body map[string]any) Reply {
	s.mu.Lock()
	defer s.mu.Unlock()
	first, last := FirstUserMessage(body), LastUserMessage(body)
	for key, script := range s.scripts {
		if strings.Contains(first, key) || strings.Contains(last, key) {
			return s.take(&script, key)
		}
	}
	if len(s.replies) == 0 {
		return Reply{FailWith: http.StatusNotFound, Text: noMoreReplies}
	}
	reply := s.replies[0]
	s.replies = s.replies[1:]
	return reply
}

func (s *Server) take(script *[]Reply, key string) Reply {
	if len(*script) == 0 {
		return Reply{FailWith: http.StatusNotFound, Text: noMoreReplies}
	}
	reply := (*script)[0]
	s.scripts[key] = (*script)[1:]
	return reply
}

// TextOf is a message's text: all of it, or its text parts when it carries pictures too.
func TextOf(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, _ := content.([]any)
	texts := []string{}
	for _, part := range parts {
		object, _ := part.(map[string]any)
		text, _ := object["text"].(string)
		texts = append(texts, text)
	}
	return strings.Join(texts, "\n")
}

// FirstUserMessage is the text of a request's first user message.
func FirstUserMessage(body map[string]any) string {
	for _, message := range Messages(body) {
		if message["role"] == "user" {
			return TextOf(message["content"])
		}
	}
	return ""
}

// LastUserMessage is the text of a request's last user message.
func LastUserMessage(body map[string]any) string {
	messages := Messages(body)
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index]["role"] == "user" {
			return TextOf(messages[index]["content"])
		}
	}
	return ""
}

// Messages is a request's messages, as maps.
func Messages(body map[string]any) []map[string]any {
	list, _ := conversationOf(body).([]any)
	messages := []map[string]any{}
	for _, each := range list {
		if message, ok := each.(map[string]any); ok {
			messages = append(messages, message)
		}
	}
	return messages
}

// JSONText is a value as JavaScript's JSON.stringify writes it: no HTML escapes.
func JSONText(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(buffer.String(), "\n")
}

// conversationOf is a request's messages, or its input items through the Responses API.
func conversationOf(body map[string]any) any {
	if messages, found := body["messages"]; found {
		return messages
	}
	return body["input"]
}

func promptTokensOf(body map[string]any) int {
	messages := conversationOf(body)
	if messages == nil {
		messages = []any{}
	}
	length := len(utf16.Encode([]rune(JSONText(messages))))
	return (length + charsPerToken - 1) / charsPerToken
}

func halves(text string) []string {
	runes := []rune(text)
	middle := (len(runes) + 1) / 2
	parts := []string{}
	for _, part := range []string{string(runes[:middle]), string(runes[middle:])} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func deltaChunk(delta map[string]any) map[string]any {
	return map[string]any{"choices": []any{map[string]any{"delta": delta, "index": 0}}, "object": "chat.completion.chunk"}
}

// chunksOf is the chunks a streaming reply arrives in: text in pieces, each tool call named first, then its
// arguments.
func chunksOf(reply Reply, promptTokens int) []any {
	chunks := []any{}
	for _, part := range halves(reply.Reasoning) {
		chunks = append(chunks, deltaChunk(map[string]any{"reasoning_content": part}))
	}
	for _, part := range halves(reply.Text) {
		chunks = append(chunks, deltaChunk(map[string]any{"content": part}))
	}
	for index, call := range reply.ToolCalls {
		text, isText := call.Arguments.(string)
		if !isText {
			text = JSONText(call.Arguments)
		}
		named := map[string]any{"function": map[string]any{"arguments": "", "name": call.Name}, "id": fmt.Sprintf("call-%d", index), "index": index}
		chunks = append(chunks, deltaChunk(map[string]any{"tool_calls": []any{named}}))
		for _, part := range halves(text) {
			chunks = append(chunks, deltaChunk(map[string]any{"tool_calls": []any{map[string]any{"function": map[string]any{"arguments": part}, "index": index}}}))
		}
	}
	finish := "stop"
	if len(reply.ToolCalls) > 0 {
		finish = "tool_calls"
	}
	chunks = append(chunks,
		map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": finish, "index": 0}}},
		map[string]any{"choices": []any{}, "usage": map[string]any{"completion_tokens": completionTokens, "prompt_tokens": promptTokens}},
	)
	return chunks
}

func stream(response http.ResponseWriter, reply Reply, promptTokens int) {
	response.Header().Set("content-type", "text/event-stream")
	response.WriteHeader(http.StatusOK)
	flusher, _ := response.(http.Flusher)
	for _, chunk := range chunksOf(reply, promptTokens) {
		fmt.Fprintf(response, "data: %s\n\n", JSONText(chunk))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(reply.ChunkDelay)
	}
	fmt.Fprint(response, "data: [DONE]\n\n")
}

// responsesEventsOf is the events a reply streams in through OpenAI's Responses API: its reasoning summary, its
// text, each call named then its arguments, and the last event with the reply's items, the reasoning encrypted.
func responsesEventsOf(reply Reply, promptTokens int) []map[string]any {
	events := []map[string]any{}
	output := []any{}
	added := func(item map[string]any) {
		events = append(events, map[string]any{"item": item, "output_index": len(output), "type": "response.output_item.added"})
	}
	deltas := func(kind, text string) {
		for _, part := range halves(text) {
			events = append(events, map[string]any{"delta": part, "output_index": len(output), "type": kind})
		}
	}
	if reply.Reasoning != "" {
		added(map[string]any{"id": "rs_0", "summary": []any{}, "type": "reasoning"})
		events = append(events, map[string]any{"output_index": 0, "summary_index": 0, "type": "response.reasoning_summary_part.added"})
		deltas("response.reasoning_summary_text.delta", reply.Reasoning)
		output = append(output, map[string]any{
			"encrypted_content": EncryptedReasoning, "id": "rs_0",
			"summary": []any{map[string]any{"text": reply.Reasoning, "type": "summary_text"}}, "type": "reasoning",
		})
	}
	if reply.Text != "" {
		added(map[string]any{"content": []any{}, "id": "msg_0", "role": "assistant", "type": "message"})
		deltas("response.output_text.delta", reply.Text)
		output = append(output, map[string]any{
			"content": []any{map[string]any{"annotations": []any{}, "text": reply.Text, "type": "output_text"}},
			"id":      "msg_0", "role": "assistant", "status": "completed", "type": "message",
		})
	}
	for index, call := range reply.ToolCalls {
		text, isText := call.Arguments.(string)
		if !isText {
			text = JSONText(call.Arguments)
		}
		item := map[string]any{"arguments": "", "call_id": fmt.Sprintf("call-%d", index), "id": fmt.Sprintf("fc_%d", index), "name": call.Name, "type": "function_call"}
		added(item)
		deltas("response.function_call_arguments.delta", text)
		item["arguments"], item["status"] = text, "completed"
		output = append(output, item)
	}
	return append(events, map[string]any{"response": map[string]any{
		"output": output, "status": "completed",
		"usage": map[string]any{"input_tokens": promptTokens, "output_tokens": completionTokens},
	}, "type": "response.completed"})
}

func streamResponses(response http.ResponseWriter, reply Reply, promptTokens int) {
	response.Header().Set("content-type", "text/event-stream")
	response.WriteHeader(http.StatusOK)
	flusher, _ := response.(http.Flusher)
	for _, event := range responsesEventsOf(reply, promptTokens) {
		fmt.Fprintf(response, "event: %s\ndata: %s\n\n", event["type"], JSONText(event))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(reply.ChunkDelay)
	}
}

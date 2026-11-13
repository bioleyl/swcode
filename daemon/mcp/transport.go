package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	// maxMessage is the longest message read from a server: a tool can give back a file.
	maxMessage = 16 << 20
	// exitGrace is how long a server has to end once its input is closed, before it is ended.
	exitGrace = 2 * time.Second
	// The headers of the HTTP transport.
	headerSession = "Mcp-Session-Id"
	headerVersion = "MCP-Protocol-Version"
	headerMethod  = "Mcp-Method"
	headerName    = "Mcp-Name"
	eventStream   = "text/event-stream"
	acceptBoth    = "application/json, text/event-stream"
	dataField     = "data:"
)

// sent is a request on its way: what it is, and what an HTTP server wants said of it beside its body.
type sent struct {
	request request
	// version and name go in headers from the newer revision on: the revision, and the tool a call is for.
	version string
	name    string
}

// transport carries messages to a server and back: one request and its answer at a time for HTTP, any number at
// once for a program's streams.
type transport interface {
	// call sends a request and waits for its answer.
	call(ctx context.Context, out sent) (message, error)
	// notify sends a message that takes no answer.
	notify(ctx context.Context, out sent) error
	close()
}

// stdio talks to a program over its standard streams: a JSON message a line, each way.
type stdio struct {
	cmd     *exec.Cmd
	input   io.WriteCloser
	writing sync.Mutex
	mu      sync.Mutex
	waiting map[int64]chan message
	// ended is closed when the program's output ends: nothing more comes.
	ended chan struct{}
}

// startStdio starts a server's program, its environment the daemon's with the server's own on top.
func startStdio(config ServerConfig) (*stdio, error) {
	cmd := exec.Command(config.Command, config.Args...)
	cmd.Env = os.Environ()
	for name, value := range config.Env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	started := &stdio{cmd: cmd, ended: make(chan struct{}), input: input, waiting: map[int64]chan message{}}
	go started.read(output)
	return started, nil
}

// read hands each line the program writes to the request it answers. A request of the program's own gets a no:
// this client offers a server nothing.
func (s *stdio) read(output io.Reader) {
	defer close(s.ended)
	lines := bufio.NewScanner(output)
	lines.Buffer(make([]byte, 0, 64<<10), maxMessage)
	for lines.Scan() {
		var got message
		if json.Unmarshal(lines.Bytes(), &got) != nil {
			continue
		}
		if got.Method != "" {
			s.refuse(got)
			continue
		}
		s.deliver(got)
	}
}

func (s *stdio) deliver(got message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, waiter := range s.waiting {
		if got.answers(id) {
			delete(s.waiting, id)
			waiter <- got
			return
		}
	}
}

// refuse answers a request a server makes of its client: a ping with nothing, anything else with a no. A
// notification takes no answer.
func (s *stdio) refuse(asked message) {
	if asked.ID == nil {
		return
	}
	answer := map[string]any{"id": asked.ID, "jsonrpc": rpcVersion}
	if asked.Method == methodPing {
		answer["result"] = map[string]any{}
	} else {
		answer["error"] = rpcError{Code: codeMethodNotFound, Message: "swcode offers a server nothing"}
	}
	_ = s.write(answer)
}

func (s *stdio) write(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	_, err = s.input.Write(append(data, '\n'))
	return err
}

func (s *stdio) call(ctx context.Context, out sent) (message, error) {
	answer := make(chan message, 1)
	id := *out.request.ID
	s.mu.Lock()
	s.waiting[id] = answer
	s.mu.Unlock()
	forget := func() {
		s.mu.Lock()
		delete(s.waiting, id)
		s.mu.Unlock()
	}
	if err := s.write(out.request); err != nil {
		forget()
		return message{}, err
	}
	select {
	case got := <-answer:
		return got, nil
	case <-s.ended:
		forget()
		return message{}, errors.New("the server's program ended")
	case <-ctx.Done():
		forget()
		return message{}, ctx.Err()
	}
}

func (s *stdio) notify(_ context.Context, out sent) error { return s.write(out.request) }

// close ends the program: its input is closed, which a server takes as the word to leave, and it is ended when it
// does not.
func (s *stdio) close() {
	_ = s.input.Close()
	select {
	case <-s.ended:
	case <-time.After(exitGrace):
		_ = s.cmd.Process.Kill()
	}
	_ = s.cmd.Wait()
}

// overHTTP talks to a server at an address: each message is a POST, answered at once or as a stream of events.
type overHTTP struct {
	client  *http.Client
	url     string
	headers map[string]string
	mu      sync.Mutex
	// session is what a server that shakes hands may give its client, to send back with every request after.
	session string
}

func (h *overHTTP) post(ctx context.Context, out sent) (*http.Response, error) {
	body, err := json.Marshal(out.request)
	if err != nil {
		return nil, err
	}
	post, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for name, value := range h.headers {
		post.Header.Set(name, value)
	}
	post.Header.Set("content-type", "application/json")
	post.Header.Set("accept", acceptBoth)
	post.Header.Set(headerMethod, out.request.Method)
	h.mu.Lock()
	session := h.session
	h.mu.Unlock()
	for name, value := range map[string]string{headerSession: session, headerVersion: out.version, headerName: out.name} {
		if value != "" {
			post.Header.Set(name, value)
		}
	}
	return h.client.Do(post)
}

func (h *overHTTP) call(ctx context.Context, out sent) (message, error) {
	response, err := h.post(ctx, out)
	if err != nil {
		return message{}, err
	}
	defer response.Body.Close()
	if session := response.Header.Get(headerSession); session != "" {
		h.mu.Lock()
		h.session = session
		h.mu.Unlock()
	}
	if strings.HasPrefix(response.Header.Get("content-type"), eventStream) {
		return answerInEvents(response.Body, *out.request.ID)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMessage))
	if err != nil {
		return message{}, err
	}
	var got message
	if json.Unmarshal(data, &got) != nil || (got.Error == nil && got.Result == nil) {
		return message{}, fmt.Errorf("the server answered %d with no JSON-RPC message", response.StatusCode)
	}
	return got, nil
}

// answerInEvents reads a stream of events for the answer to a request: the server's own notifications come before
// it, and are passed over.
func answerInEvents(body io.Reader, id int64) (message, error) {
	lines := bufio.NewScanner(body)
	lines.Buffer(make([]byte, 0, 64<<10), maxMessage)
	for lines.Scan() {
		data, isData := strings.CutPrefix(lines.Text(), dataField)
		if !isData {
			continue
		}
		var got message
		if json.Unmarshal([]byte(strings.TrimSpace(data)), &got) == nil && got.answers(id) {
			return got, nil
		}
	}
	return message{}, errors.New("the server's stream ended without an answer")
}

func (h *overHTTP) notify(ctx context.Context, out sent) error {
	response, err := h.post(ctx, out)
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (h *overHTTP) close() {}

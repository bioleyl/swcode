package llm

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"swcode/daemon/protocol"
)

// Server is one llm-server of the settings: what it says of its models, and the requests this daemon sends there.
type Server struct {
	Status *StatusReader
	source protocol.LlmSource
	model  *ScheduledModel
}

// Servers is the llm-servers of the settings, by source: a request goes to the one its target names. They change
// while the daemon runs; a source whose address and key stay the same keeps its prompt caches, and the requests
// running there.
type Servers struct {
	mu   sync.Mutex
	http *http.Client
	byID map[string]*Server
}

func NewServers(httpClient *http.Client) *Servers {
	return &Servers{byID: map[string]*Server{}, http: httpClient}
}

// Configure sets the sources: a new one starts, a removed one goes, and a changed one starts again.
func (s *Servers) Configure(sources []protocol.LlmSource) {
	s.mu.Lock()
	defer s.mu.Unlock()
	servers := map[string]*Server{}
	for _, source := range sources {
		known, found := s.byID[source.ID]
		if found && sameServer(known.source, source) {
			known.source = source
			servers[source.ID] = known
			continue
		}
		servers[source.ID] = s.start(source)
	}
	s.byID = servers
}

// sameServer: only its name changed, which the requests do not depend on.
func sameServer(known, source protocol.LlmSource) bool {
	return known.URL == source.URL && known.APIKey == source.APIKey
}

func (s *Servers) start(source protocol.LlmSource) *Server {
	options := Options{APIKey: source.APIKey, HTTP: s.http, URL: source.URL}
	status := NewStatusReader(options)
	model := NewScheduledModel(clientFor(options), NewScheduler(status.SlotsOf))
	return &Server{Status: status, model: model, source: source}
}

// clientFor is the API a server is asked through: OpenAI's Responses API for OpenAI's own, Chat Completions
// otherwise.
func clientFor(options Options) Model {
	if UsesResponses(options.URL) {
		return NewResponsesClient(options)
	}
	return NewClient(options)
}

// Server is the llm-server of a source, when the settings have it.
func (s *Servers) Server(source string) (*Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	server, found := s.byID[source]
	return server, found
}

// Complete asks the llm-server the request's target names.
func (s *Servers) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	server, found := s.Server(request.Target.Source)
	if !found {
		return Reply{}, fmt.Errorf("No llm-server %q in the settings: pick another preset.", request.Target.Source)
	}
	return server.model.Complete(ctx, request, onPart)
}

// Forget is an agent done for good, on the llm-server its identity names: its slot goes to the next agent first.
func (s *Servers) Forget(agent Identity) {
	server, found := s.Server(agent.Source)
	if !found {
		return
	}
	server.model.Forget(agent)
}

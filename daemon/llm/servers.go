package llm

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

// Server is one llm-server of the settings: what it says of its models, and the slots this daemon hands out there.
type Server struct {
	Status *StatusReader
	Slots  *Scheduler
	source protocol.LlmSource
	model  *ScheduledModel
	// unsubscribe stops hearing its slots, once the server leaves the settings.
	unsubscribe func()
}

// Servers is the llm-servers of the settings, by source: a request goes to the one its target names. They change
// while the daemon runs; a source whose address, key and share stay the same keeps its slots, and the requests
// holding them.
type Servers struct {
	mu        sync.Mutex
	http      *http.Client
	byID      map[string]*Server
	order     []string
	listeners map[int]func()
	next      int
}

func NewServers(httpClient *http.Client) *Servers {
	return &Servers{byID: map[string]*Server{}, http: httpClient, listeners: map[int]func(){}}
}

// Configure sets the sources: a new one starts, a removed one goes, and a changed one starts again with new slots.
func (s *Servers) Configure(sources []protocol.LlmSource) {
	s.mu.Lock()
	servers := map[string]*Server{}
	order := []string{}
	for _, source := range sources {
		order = append(order, source.ID)
		known, found := s.byID[source.ID]
		if found && sameServer(known.source, source) {
			known.source = source
			servers[source.ID] = known
			continue
		}
		servers[source.ID] = s.start(source)
	}
	for id, known := range s.byID {
		if servers[id] != known {
			known.unsubscribe()
		}
	}
	s.byID, s.order = servers, order
	s.mu.Unlock()
	s.changed()
}

// sameServer: only its name changed, which the slots do not depend on.
func sameServer(known, source protocol.LlmSource) bool {
	return known.URL == source.URL && known.APIKey == source.APIKey && known.SlotShare == source.SlotShare
}

func (s *Servers) start(source protocol.LlmSource) *Server {
	options := Options{APIKey: source.APIKey, HTTP: s.http, URL: source.URL}
	status := NewStatusReader(options)
	// The settings were checked before they came here: a share that does not read takes every slot.
	share, _ := config.ParseSlotShare(source.SlotShare)
	slots := NewScheduler(status.SlotsOf, share)
	server := &Server{Slots: slots, Status: status, model: NewScheduledModel(NewClient(options), slots), source: source}
	server.unsubscribe = slots.Subscribe(s.changed)
	return server
}

// Server is the llm-server of a source, when the settings have it.
func (s *Servers) Server(source string) (*Server, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	server, found := s.byID[source]
	return server, found
}

// Complete asks the llm-server the request's target names, on a slot of its model.
func (s *Servers) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	server, found := s.Server(request.Target.Source)
	if !found {
		return Reply{}, fmt.Errorf("No llm-server %q in the settings: pick another preset.", request.Target.Source)
	}
	return server.model.Complete(ctx, request, onPart)
}

// Usage is how busy each model's slots are on every llm-server, in the settings' order, for the side panel.
func (s *Servers) Usage() []protocol.SlotUsage {
	s.mu.Lock()
	servers := []*Server{}
	for _, id := range s.order {
		servers = append(servers, s.byID[id])
	}
	names := map[*Server]string{}
	for _, server := range servers {
		names[server] = server.source.Name
	}
	s.mu.Unlock()
	usage := []protocol.SlotUsage{}
	for _, server := range servers {
		for _, model := range server.Slots.Usage() {
			model.Source = names[server]
			usage = append(usage, model)
		}
	}
	return usage
}

// Subscribe calls listener at every change of the slots or of the servers, until the returned function is called.
func (s *Servers) Subscribe(listener func()) func() {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	s.next++
	s.listeners[id] = listener
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.listeners, id)
	}
}

func (s *Servers) changed() {
	s.mu.Lock()
	listeners := []func(){}
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()
	for _, listener := range listeners {
		listener()
	}
}

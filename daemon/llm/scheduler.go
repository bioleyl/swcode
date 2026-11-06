package llm

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// ErrStopped is a request let go while it waited for a free slot of its model.
var ErrStopped = errors.New("Stopped while waiting for the model.")

type waiter struct {
	agentID string
	grant   chan int
}

// pool is one model's slots: which agent holds each busy one, which agent last held each, and who waits.
type pool struct {
	total      int
	busy       map[int]string
	lastHolder map[int]string
	waiting    []*waiter
}

func (p *pool) free() []int {
	free := []int{}
	for slot := range p.total {
		if _, busy := p.busy[slot]; !busy {
			free = append(free, slot)
		}
	}
	return free
}

// pick is the free slot for an agent: the one it held last, where its conversation is still cached; else one no
// other agent's cache is waiting in; else any.
func (p *pool) pick(agentID string) (int, bool) {
	free := p.free()
	for _, slot := range free {
		if p.lastHolder[slot] == agentID {
			return slot, true
		}
	}
	for _, slot := range free {
		if _, held := p.lastHolder[slot]; !held {
			return slot, true
		}
	}
	if len(free) > 0 {
		return free[0], true
	}
	return 0, false
}

// sizing is a pool being sized by llm-server, which callers asking at the same time wait for together; no pool
// when the server does not say its slots.
type sizing struct {
	done chan struct{}
	pool *pool
}

// Lease is a slot a request holds while it runs; released once, whatever happens. NoSlot when the server picks.
type Lease struct {
	Slot    int
	release func()
	once    sync.Once
}

func (l *Lease) Release() { l.once.Do(l.release) }

// passThrough is the lease of a server that does not say its slots: it picks one and queues the requests itself.
func passThrough() *Lease {
	return &Lease{Slot: NoSlot, release: func() {}}
}

// Scheduler hands out llama.cpp's slots, one request per slot, so agents running at the same time each keep their
// own prompt cache. A request waits when every slot of its model is busy, first come, first served. A server that
// does not say how many slots a model has (another than llama.cpp, or a router's model not loaded yet) gets the
// requests as they come: it queues and caches them itself, and is asked again at the next request.
type Scheduler struct {
	mu         sync.Mutex
	capacityOf func(ctx context.Context, model string) int
	sizings    map[string]*sizing
}

// NewScheduler hands out slots, asking capacityOf how many a model has (0 when llm-server does not say).
func NewScheduler(capacityOf func(ctx context.Context, model string) int) *Scheduler {
	return &Scheduler{capacityOf: capacityOf, sizings: map[string]*sizing{}}
}

// Acquire gives an agent a slot of a model, waiting for one when all are busy.
func (s *Scheduler) Acquire(ctx context.Context, model, agentID string) (*Lease, error) {
	found := s.poolOf(ctx, model)
	if found == nil {
		return passThrough(), nil
	}
	s.mu.Lock()
	if slot, ok := found.pick(agentID); ok {
		lease := s.take(found, slot, agentID)
		s.mu.Unlock()
		return lease, nil
	}
	// The turn may have stopped while the pool was being sized.
	if ctx.Err() != nil {
		s.mu.Unlock()
		return nil, ErrStopped
	}
	entry := &waiter{agentID: agentID, grant: make(chan int, 1)}
	found.waiting = append(found.waiting, entry)
	s.mu.Unlock()
	select {
	case slot := <-entry.grant:
		return s.leaseOf(found, slot), nil
	case <-ctx.Done():
		return nil, s.giveUp(found, entry)
	}
}

// giveUp takes a waiter out of the line; one granted a slot meanwhile hands it on.
func (s *Scheduler) giveUp(found *pool, entry *waiter) error {
	s.mu.Lock()
	index := slices.Index(found.waiting, entry)
	if index >= 0 {
		found.waiting = slices.Delete(found.waiting, index, index+1)
		s.mu.Unlock()
		return ErrStopped
	}
	s.mu.Unlock()
	s.leaseOf(found, <-entry.grant).Release()
	return ErrStopped
}

// poolOf is a model's pool, sized once by llm-server; nil, and asked again next time, when it does not say.
func (s *Scheduler) poolOf(ctx context.Context, model string) *pool {
	s.mu.Lock()
	known, ok := s.sizings[model]
	if ok {
		s.mu.Unlock()
		<-known.done
		return known.pool
	}
	known = &sizing{done: make(chan struct{})}
	s.sizings[model] = known
	s.mu.Unlock()
	total := s.capacityOf(ctx, model)
	s.mu.Lock()
	if total > 0 {
		known.pool = &pool{busy: map[int]string{}, lastHolder: map[int]string{}, total: total}
	} else {
		delete(s.sizings, model)
	}
	s.mu.Unlock()
	close(known.done)
	return known.pool
}

// take marks a slot busy; the caller holds the lock.
func (s *Scheduler) take(found *pool, slot int, agentID string) *Lease {
	found.busy[slot] = agentID
	found.lastHolder[slot] = agentID
	return s.leaseOf(found, slot)
}

func (s *Scheduler) leaseOf(found *pool, slot int) *Lease {
	return &Lease{Slot: slot, release: func() { s.free(found, slot) }}
}

// free hands a freed slot to the request that waited longest.
func (s *Scheduler) free(found *pool, slot int) {
	s.mu.Lock()
	delete(found.busy, slot)
	if len(found.waiting) == 0 {
		s.mu.Unlock()
		return
	}
	next := found.waiting[0]
	found.waiting = found.waiting[1:]
	s.take(found, slot, next.agentID)
	s.mu.Unlock()
	next.grant <- slot
}

// ScheduledModel is a model whose requests each hold a slot while they run: the agent waits for one when all are
// busy.
type ScheduledModel struct {
	model Model
	slots *Scheduler
}

func NewScheduledModel(model Model, slots *Scheduler) *ScheduledModel {
	return &ScheduledModel{model: model, slots: slots}
}

func (m *ScheduledModel) Complete(ctx context.Context, request Request, onPart func(Part)) (Reply, error) {
	lease, err := m.slots.Acquire(ctx, request.Target.Model, request.Target.AgentID)
	if err != nil {
		return Reply{}, err
	}
	defer lease.Release()
	request.Target.Slot = lease.Slot
	return m.model.Complete(ctx, request, onPart)
}

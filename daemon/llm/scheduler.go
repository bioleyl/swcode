package llm

import (
	"context"
	"errors"
	"slices"
	"sync"

	"swcode/daemon/config"
	"swcode/daemon/protocol"
)

// A server that does not say how many slots it has answers one request at a time.
const defaultSlots = 1

// ErrStopped is a request let go while it waited for a free slot.
var ErrStopped = errors.New("Stopped while waiting for a free slot.")

// SharedSlots is the slots of a share: its part of the model's, at least one. With fewer slots than parts, some
// shares use the same slot.
func SharedSlots(total int, share config.SlotShare) []int {
	start := (share.Part - 1) * total / share.Parts
	end := min(max(share.Part*total/share.Parts, start+1), total)
	slots := []int{}
	for slot := start; slot < end; slot++ {
		slots = append(slots, slot)
	}
	return slots
}

type waiter struct {
	agentID string
	grant   chan int
}

// pool is one model's slots: which agent holds each busy one, which agent last held each, and who waits.
type pool struct {
	model string
	// slots are the ones this daemon uses, its share of the model's.
	slots []int
	// total is how many slots the model has, every daemon's.
	total int
	// provisional: the count is a guess, llama-server did not say, and the pool is asked again once it is idle.
	provisional bool
	busy        map[int]string
	lastHolder  map[int]string
	waiting     []*waiter
}

func (p *pool) free() []int {
	free := []int{}
	for _, slot := range p.slots {
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

// sizing is a pool being sized by llama-server, which callers asking at the same time wait for together.
type sizing struct {
	done chan struct{}
	pool *pool
}

// Lease is a slot a request holds while it runs; released once, whatever happens.
type Lease struct {
	Slot    int
	release func()
	once    sync.Once
}

func (l *Lease) Release() { l.once.Do(l.release) }

// Scheduler hands out llama-server's slots, one request per slot, so agents running at the same time each keep
// their own prompt cache. A request waits when every slot of its model is busy, first come, first served. When
// several daemons share a llama-server, each hands out its share of the slots only.
type Scheduler struct {
	mu         sync.Mutex
	capacityOf func(ctx context.Context, model string) int
	share      config.SlotShare
	sizings    map[string]*sizing
	ready      []*pool
	listeners  map[int]func()
	next       int
}

// NewScheduler hands out slots, asking capacityOf how many a model has (0 when llama-server does not say).
func NewScheduler(capacityOf func(ctx context.Context, model string) int, share config.SlotShare) *Scheduler {
	return &Scheduler{capacityOf: capacityOf, listeners: map[int]func(){}, share: share, sizings: map[string]*sizing{}}
}

// Acquire gives an agent a slot of a model, waiting for one when all are busy.
func (s *Scheduler) Acquire(ctx context.Context, model, agentID string) (*Lease, error) {
	found := s.poolOf(ctx, model)
	s.mu.Lock()
	if slot, ok := found.pick(agentID); ok {
		lease := s.take(found, slot, agentID)
		s.mu.Unlock()
		s.changed()
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

// Usage is how busy each model's slots are, for the side panel.
func (s *Scheduler) Usage() []protocol.SlotUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage := []protocol.SlotUsage{}
	for _, each := range s.ready {
		busy := []int{}
		for slot := range each.busy {
			busy = append(busy, slot)
		}
		slices.Sort(busy)
		usage = append(usage, protocol.SlotUsage{
			Busy: busy, Model: each.model, Own: append([]int{}, each.slots...), Total: each.total,
		})
	}
	return usage
}

// Subscribe calls listener at every change of the slots, until the returned function is called.
func (s *Scheduler) Subscribe(listener func()) func() {
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
	answer := s.capacityOf(ctx, model)
	total := answer
	if total <= 0 {
		total = defaultSlots
	}
	created := &pool{
		busy: map[int]string{}, lastHolder: map[int]string{}, model: model,
		provisional: answer <= 0, slots: SharedSlots(total, s.share), total: total,
	}
	s.mu.Lock()
	s.ready = slices.DeleteFunc(s.ready, func(each *pool) bool { return each.model == model })
	s.ready = append(s.ready, created)
	known.pool = created
	s.mu.Unlock()
	close(known.done)
	return created
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
	if len(found.waiting) > 0 {
		next := found.waiting[0]
		found.waiting = found.waiting[1:]
		s.take(found, slot, next.agentID)
		s.mu.Unlock()
		next.grant <- slot
		s.changed()
		return
	}
	s.forgetGuess(found)
	s.mu.Unlock()
	s.changed()
}

// forgetGuess lets go of a pool sized by a guess once idle, so the next request asks llama-server again; the
// panels keep showing it until then. The caller holds the lock.
func (s *Scheduler) forgetGuess(found *pool) {
	if !found.provisional || len(found.busy) > 0 {
		return
	}
	if known, ok := s.sizings[found.model]; ok && known.pool == found {
		delete(s.sizings, found.model)
	}
}

func (s *Scheduler) changed() {
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

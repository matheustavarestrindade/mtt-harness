package permission

import (
	"context"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Broker struct {
	mu        sync.Mutex
	pending   map[string]chan atom.PermissionDecision
	timeout   time.Duration
	onRequest func(request atom.PermissionRequest)
}

func NewBroker(timeout time.Duration) *Broker {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &Broker{
		pending: map[string]chan atom.PermissionDecision{},
		timeout: timeout,
	}
}

func (b *Broker) SetRequestHandler(handler func(request atom.PermissionRequest)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onRequest = handler
}

func (b *Broker) Request(ctx context.Context, request atom.PermissionRequest) (atom.PermissionDecision, error) {
	channel := make(chan atom.PermissionDecision, 1)
	b.mu.Lock()
	b.pending[request.ID] = channel
	handler := b.onRequest
	b.mu.Unlock()
	if handler != nil {
		handler(request)
	}
	select {
	case decision := <-channel:
		return decision, nil
	case <-time.After(b.timeout):
		return atom.PermissionDecision{RequestID: request.ID, Kind: atom.VerdictDeny, Scope: atom.ScopeOnce}, nil
	case <-ctx.Done():
		return atom.PermissionDecision{RequestID: request.ID, Kind: atom.VerdictDeny, Scope: atom.ScopeOnce}, nil
	}
}

func (b *Broker) Resolve(id string, decision atom.PermissionDecision) bool {
	b.mu.Lock()
	channel, ok := b.pending[id]
	if ok {
		delete(b.pending, id)
	}
	b.mu.Unlock()
	if !ok {
		return false
	}
	decision.RequestID = id
	channel <- decision
	return true
}

func (b *Broker) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

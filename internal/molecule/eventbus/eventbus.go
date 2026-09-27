package eventbus

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Bus struct {
	mu       sync.RWMutex
	handlers map[atom.EventName][]harness.Handler
	seq      uint64
}

func New() *Bus {
	return &Bus{handlers: map[atom.EventName][]harness.Handler{}}
}

func (b *Bus) Send(ctx context.Context, event atom.Event) {
	b.mu.Lock()
	b.seq++
	event.Seq = b.seq
	list := append([]harness.Handler(nil), b.handlers[event.Name]...)
	b.mu.Unlock()
	for _, handler := range list {
		handler(ctx, event)
	}
}

func (b *Bus) On(name atom.EventName, handler harness.Handler) harness.Unsubscribe {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[name] = append(b.handlers[name], handler)
	index := len(b.handlers[name]) - 1
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.handlers[name]
		if index < len(list) {
			b.handlers[name] = append(list[:index], list[index+1:]...)
		}
	}
}

func (b *Bus) LastSequence() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.seq
}

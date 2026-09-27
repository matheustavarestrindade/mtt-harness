package eventbus

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Recorder func(ctx context.Context, event atom.Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[atom.EventName][]harness.Handler
	recorder Recorder
	seq      uint64
}

func New() *Bus {
	return &Bus{handlers: map[atom.EventName][]harness.Handler{}}
}

func (b *Bus) SetRecorder(recorder Recorder) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.recorder = recorder
}

func (b *Bus) Send(ctx context.Context, event atom.Event) {
	b.mu.Lock()
	b.seq++
	event.Seq = b.seq
	list := append([]harness.Handler(nil), b.handlers[event.Name]...)
	all := append([]harness.Handler(nil), b.handlers[atom.EventName("*")]...)
	recorder := b.recorder
	b.mu.Unlock()
	if recorder != nil {
		recorder(ctx, event)
	}
	for _, handler := range list {
		handler(ctx, event)
	}
	for _, handler := range all {
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

package harness

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Unsubscribe func()

type Handler func(ctx context.Context, event atom.Event)
type Middleware[T any] func(ctx context.Context, value T) (T, error)
type Decision[T any] func(ctx context.Context, value T) (atom.Verdict, error)

type Harness struct {
	mu        sync.RWMutex
	events    map[atom.EventName][]Handler
	pipes     map[string]any
	decisions map[string]any
	tools     map[string]Tool
	providers map[string]Provider
	watchers  []ProcessWatcher
}

func New() *Harness {
	return &Harness{
		events:    map[atom.EventName][]Handler{},
		pipes:     map[string]any{},
		decisions: map[string]any{},
		tools:     map[string]Tool{},
		providers: map[string]Provider{},
	}
}

func (h *Harness) On(event atom.EventName, handler Handler) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events[event] = append(h.events[event], handler)
	index := len(h.events[event]) - 1
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		list := h.events[event]
		if index < len(list) {
			h.events[event] = append(list[:index], list[index+1:]...)
		}
	}
}

func Emit(ctx context.Context, h *Harness, event atom.Event) {
	h.mu.RLock()
	list := append([]Handler(nil), h.events[event.Name]...)
	h.mu.RUnlock()
	for _, handler := range list {
		handler(ctx, event)
	}
}

func Pipe[T any](h *Harness, stage atom.Stage[T], fn Middleware[T]) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, _ := h.pipes[stage.Name].([]Middleware[T])
	list = append(list, fn)
	h.pipes[stage.Name] = list
	index := len(list) - 1
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		current, _ := h.pipes[stage.Name].([]Middleware[T])
		if index < len(current) {
			h.pipes[stage.Name] = append(current[:index], current[index+1:]...)
		}
	}
}

func Run[T any](ctx context.Context, h *Harness, stage atom.Stage[T], value T) (T, error) {
	h.mu.RLock()
	list, _ := h.pipes[stage.Name].([]Middleware[T])
	h.mu.RUnlock()
	var err error
	for _, fn := range list {
		value, err = fn(ctx, value)
		if err != nil {
			return value, err
		}
	}
	return value, nil
}

func Decide[T any](h *Harness, stage atom.Stage[T], fn Decision[T]) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	list, _ := h.decisions[stage.Name].([]Decision[T])
	list = append(list, fn)
	h.decisions[stage.Name] = list
	index := len(list) - 1
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		current, _ := h.decisions[stage.Name].([]Decision[T])
		if index < len(current) {
			h.decisions[stage.Name] = append(current[:index], current[index+1:]...)
		}
	}
}

func Check[T any](ctx context.Context, h *Harness, stage atom.Stage[T], value T) (atom.Verdict, error) {
	h.mu.RLock()
	list, _ := h.decisions[stage.Name].([]Decision[T])
	h.mu.RUnlock()
	verdict := atom.Verdict{Kind: atom.VerdictAllow}
	for _, fn := range list {
		found, err := fn(ctx, value)
		if err != nil {
			return found, err
		}
		if found.Kind == atom.VerdictDeny {
			return found, nil
		}
		if found.Kind == atom.VerdictAsk {
			verdict = found
		}
	}
	return verdict, nil
}

func (h *Harness) Tool(tool Tool) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.tools[tool.Name()] = tool
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.tools, tool.Name())
	}
}

func (h *Harness) ToolByName(name string) (Tool, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	tool, ok := h.tools[name]
	return tool, ok
}

func (h *Harness) Tools() []Tool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]Tool, 0, len(h.tools))
	for _, tool := range h.tools {
		list = append(list, tool)
	}
	return list
}

func (h *Harness) Provider(provider Provider) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.providers[provider.Name()] = provider
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.providers, provider.Name())
	}
}

func (h *Harness) Providers() []Provider {
	h.mu.RLock()
	defer h.mu.RUnlock()
	list := make([]Provider, 0, len(h.providers))
	for _, provider := range h.providers {
		list = append(list, provider)
	}
	return list
}

func (h *Harness) Watch(watcher ProcessWatcher) Unsubscribe {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.watchers = append(h.watchers, watcher)
	index := len(h.watchers) - 1
	return func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if index < len(h.watchers) {
			h.watchers = append(h.watchers[:index], h.watchers[index+1:]...)
		}
	}
}

func (h *Harness) Watchers() []ProcessWatcher {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return append([]ProcessWatcher(nil), h.watchers...)
}

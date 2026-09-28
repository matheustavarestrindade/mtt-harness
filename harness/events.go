package harness

import (
	"context"
	"sort"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (harnessRuntime *Harness) On(name atom.EventName, handler Handler) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	harnessRuntime.events[name] = append(harnessRuntime.events[name], registration[Handler]{identifier, handler})
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		harnessRuntime.events[name] = withoutRegistration(harnessRuntime.events[name], identifier)
	}
}

// Emit takes a snapshot before invoking callbacks. Callbacks can register or
// remove handlers without acquiring a lock already held by the dispatcher.
func (harnessRuntime *Harness) Emit(operationContext context.Context, event atom.Event) {
	harnessRuntime.mutex.RLock()
	handlers := append([]registration[Handler](nil), harnessRuntime.events[event.Name]...)
	if event.Name != "*" {
		handlers = append(handlers, harnessRuntime.events["*"]...)
	}
	harnessRuntime.mutex.RUnlock()
	sort.Slice(handlers, func(first, second int) bool {
		return handlers[first].identifier < handlers[second].identifier
	})
	for _, handler := range handlers {
		handler.value(operationContext, event)
	}
}

package harness

import (
	"context"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Unsubscribe removes a registered callback or capability.
type Unsubscribe func()

// Handler observes an event without changing its payload.
type Handler func(operationContext context.Context, event atom.Event)

// Middleware transforms a stage value; an error stops the stage pipeline.
type Middleware[Value any] func(operationContext context.Context, value Value) (Value, error)

// Decision supplies a policy verdict for a stage value.
type Decision[Value any] func(operationContext context.Context, value Value) (atom.Verdict, error)

// Harness owns plugin registrations. Runtime services must use these
// registrations when executing capabilities and delivering events.
type Harness struct {
	mutex     sync.RWMutex
	nextID    uint64
	events    map[atom.EventName][]registration[Handler]
	pipes     map[string]any
	decisions map[string]any
	tools     map[string]registration[Tool]
	providers map[string]registration[Provider]
	watchers  []registration[ProcessWatcher]
}

func New() *Harness {
	return &Harness{
		events:    map[atom.EventName][]registration[Handler]{},
		pipes:     map[string]any{},
		decisions: map[string]any{},
		tools:     map[string]registration[Tool]{},
		providers: map[string]registration[Provider]{},
	}
}

type registration[Value any] struct {
	identifier uint64
	value      Value
}

// The caller holds mutex. IDs remain stable when earlier registrations leave.
func (harnessRuntime *Harness) nextRegistrationID() uint64 {
	harnessRuntime.nextID++
	return harnessRuntime.nextID
}

func withoutRegistration[Value any](entries []registration[Value], identifier uint64) []registration[Value] {
	result := make([]registration[Value], 0, len(entries))
	for _, entry := range entries {
		if entry.identifier != identifier {
			result = append(result, entry)
		}
	}
	return result
}

package harness

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func Pipe[Value any](harnessRuntime *Harness, stage atom.Stage[Value], middleware Middleware[Value]) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	entries, _ := harnessRuntime.pipes[stage.Name].([]registration[Middleware[Value]])
	harnessRuntime.pipes[stage.Name] = append(entries, registration[Middleware[Value]]{identifier, middleware})
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		entries, _ := harnessRuntime.pipes[stage.Name].([]registration[Middleware[Value]])
		harnessRuntime.pipes[stage.Name] = withoutRegistration(entries, identifier)
	}
}

func Run[Value any](operationContext context.Context, harnessRuntime *Harness, stage atom.Stage[Value], value Value) (Value, error) {
	harnessRuntime.mutex.RLock()
	entries, _ := harnessRuntime.pipes[stage.Name].([]registration[Middleware[Value]])
	entries = append([]registration[Middleware[Value]](nil), entries...)
	harnessRuntime.mutex.RUnlock()
	for _, entry := range entries {
		if operationError := operationContext.Err(); operationError != nil {
			return value, operationError
		}
		var operationError error
		value, operationError = entry.value(operationContext, value)
		if operationError != nil {
			return value, operationError
		}
	}
	return value, operationContext.Err()
}

func Decide[Value any](harnessRuntime *Harness, stage atom.Stage[Value], decision Decision[Value]) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	entries, _ := harnessRuntime.decisions[stage.Name].([]registration[Decision[Value]])
	harnessRuntime.decisions[stage.Name] = append(entries, registration[Decision[Value]]{identifier, decision})
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		entries, _ := harnessRuntime.decisions[stage.Name].([]registration[Decision[Value]])
		harnessRuntime.decisions[stage.Name] = withoutRegistration(entries, identifier)
	}
}

func Check[Value any](operationContext context.Context, harnessRuntime *Harness, stage atom.Stage[Value], value Value) (atom.Verdict, error) {
	harnessRuntime.mutex.RLock()
	entries, _ := harnessRuntime.decisions[stage.Name].([]registration[Decision[Value]])
	entries = append([]registration[Decision[Value]](nil), entries...)
	harnessRuntime.mutex.RUnlock()
	verdict := atom.Verdict{Kind: atom.VerdictAllow}
	for _, entry := range entries {
		if operationError := operationContext.Err(); operationError != nil {
			return verdict, operationError
		}
		decision, operationError := entry.value(operationContext, value)
		if operationError != nil {
			return decision, operationError
		}
		switch decision.Kind {
		case atom.VerdictDeny:
			return decision, nil
		case atom.VerdictAsk:
			verdict = decision
		case atom.VerdictAllow:
		default:
			return atom.Verdict{}, fmt.Errorf("stage %s returned an invalid verdict %q", stage.Name, decision.Kind)
		}
	}
	return verdict, operationContext.Err()
}

package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Process owns a command whose lifetime can exceed one turn.
type Process interface {
	ID() string
	PID() int
	Write(data []byte) error
	Kill(signal atom.Signal) error
	Wait() (atom.ExitStatus, error)
	Events() <-chan atom.ProcessEvent
}

// InteractiveProcess accepts bounded input without extending the command's
// lifetime. Closing a viewer does not close the process input or stop the command.
type InteractiveProcess interface {
	Process
	WriteContext(context.Context, []byte) error
	ResizeTerminal(columns, rows int) error
	HasTerminal() bool
	Running() bool
}

type ProcessWatcher interface {
	Match(event atom.ProcessEvent) bool
	OnMatch(operationContext context.Context, event atom.ProcessEvent)
}

func (harnessRuntime *Harness) Watch(watcher ProcessWatcher) Unsubscribe {
	harnessRuntime.mutex.Lock()
	identifier := harnessRuntime.nextRegistrationID()
	harnessRuntime.watchers = append(harnessRuntime.watchers, registration[ProcessWatcher]{identifier, watcher})
	harnessRuntime.mutex.Unlock()
	return func() {
		harnessRuntime.mutex.Lock()
		defer harnessRuntime.mutex.Unlock()
		harnessRuntime.watchers = withoutRegistration(harnessRuntime.watchers, identifier)
	}
}

func (harnessRuntime *Harness) Watchers() []ProcessWatcher {
	harnessRuntime.mutex.RLock()
	defer harnessRuntime.mutex.RUnlock()
	watchers := make([]ProcessWatcher, 0, len(harnessRuntime.watchers))
	for _, entry := range harnessRuntime.watchers {
		watchers = append(watchers, entry.value)
	}
	return watchers
}

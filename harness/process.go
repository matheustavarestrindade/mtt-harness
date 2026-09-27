package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Process interface {
	ID() string
	PID() int
	Write(data []byte) error
	Kill(signal atom.Signal) error
	Wait() (atom.ExitStatus, error)
	Events() <-chan atom.ProcessEvent
}

type ProcessWatcher interface {
	Match(event atom.ProcessEvent) bool
	OnMatch(ctx context.Context, event atom.ProcessEvent)
}

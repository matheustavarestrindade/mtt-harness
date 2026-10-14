package process

import (
	"context"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Activity is an atomic observation of command completion and output activity.
// LastOutputAt is the start time until the first nonempty output write.
type Activity struct {
	Running      bool
	LastOutputAt time.Time
	ObservedAt   time.Time
	Stdout       []byte
	Stderr       []byte
}

func (processHandle *Handle) Activity() Activity {
	processHandle.mutex.Lock()
	defer processHandle.mutex.Unlock()
	lastOutputAt := processHandle.lastOutputAt
	if lastOutputAt.IsZero() {
		lastOutputAt = processHandle.startEvent.At
	}
	return Activity{Running: !processHandle.completed, LastOutputAt: lastOutputAt, ObservedAt: time.Now(), Stdout: append([]byte(nil), processHandle.stdout...), Stderr: append([]byte(nil), processHandle.stderr...)}
}

// WaitForExitOrIdle leaves the process alive on inactivity or cancellation.
// Timers consult the output owner's timestamp, not lossy event subscriptions.
func (processHandle *Handle) WaitForExitOrIdle(operationContext context.Context, idleTimeout time.Duration) (atom.ExitStatus, *Activity, error) {
	var timer *time.Timer
	var idle <-chan time.Time
	if idleTimeout > 0 {
		timer = time.NewTimer(idleTimeout)
		idle = timer.C
		defer timer.Stop()
	}
	for {
		select {
		case <-operationContext.Done():
			return atom.ExitStatus{}, nil, operationContext.Err()
		case <-processHandle.done:
			status, operationError := processHandle.Wait()
			return status, nil, operationError
		case <-idle:
			activity := processHandle.Activity()
			if !activity.Running {
				status, operationError := processHandle.Wait()
				return status, nil, operationError
			}
			remaining := idleTimeout - activity.ObservedAt.Sub(activity.LastOutputAt)
			if remaining > 0 {
				timer.Reset(remaining)
				continue
			}
			return atom.ExitStatus{}, &activity, nil
		}
	}
}

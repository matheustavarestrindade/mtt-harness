package loop

import (
	"context"
	"errors"
	"fmt"
	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var errQueuedAdmission = errors.New("admit queued messages")

// Context fitting may remove older turns, but must not silently discard input
// just admitted for this request. An oversized batch remains saved as an error.
func validateAdmittedInput(messages []atom.Message, identifiers []string) error {
	if len(identifiers) == 0 {
		return nil
	}
	present := make(map[string]bool, len(messages))
	for _, message := range messages {
		present[message.ID] = true
	}
	for _, identifier := range identifiers {
		if !present[identifier] {
			return fmt.Errorf("queued message %q was excluded from the model request", identifier)
		}
	}
	return nil
}

// The model worker waits for accepted admission to finish even after cancellation.
// This joins an in-flight commit and returns its exact IDs for durable cleanup.
// The coordinator owns batch selection; only its I/O worker changes storage.
func (coordinator *sessionCoordinator) admitInput(operationContext context.Context) ([]string, error) {
	command := sessionCommand{kind: admitQueuedMessages, operationContext: operationContext, reply: make(chan sessionReply, 1)}
	select {
	case <-operationContext.Done():
		return nil, operationContext.Err()
	case <-coordinator.done:
		return nil, ErrQueueClosed
	case coordinator.commands <- command:
	}
	select {
	case response := <-command.reply:
		return response.messageIDs, response.operationError
	case <-coordinator.done:
		return nil, ErrQueueClosed
	}
}

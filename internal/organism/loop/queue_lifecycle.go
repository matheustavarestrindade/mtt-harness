package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (messageQueue *Queue) Revert(operationContext context.Context, session atom.Session, messageID string) (int, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return 0, operationError
	}
	response, operationError := coordinator.ask(operationContext, sessionCommand{kind: revertConversation, messageID: messageID})
	return response.removed, operationError
}

func (messageQueue *Queue) StopInstance(operationContext context.Context, instanceID string) error {
	_, operationError := messageQueue.ask(operationContext, queueCommand{kind: stopInstance, instanceID: instanceID})
	return operationError
}

func (messageQueue *Queue) ResumeInstance(operationContext context.Context, instanceID string) error {
	_, operationError := messageQueue.ask(operationContext, queueCommand{kind: resumeInstance, instanceID: instanceID})
	return operationError
}

// Restore runs before serving requests. Recovery I/O and event handlers execute
// outside the directory owner, so observers can query status during recovery.
func (messageQueue *Queue) Restore(operationContext context.Context) error {
	_, operationError := messageQueue.ask(operationContext, queueCommand{kind: restoreQueue})
	return operationError
}

// Close is irreversible once accepted. A deadline stops this caller's wait;
// owners continue draining their workers and subsequent Close calls can join.
func (messageQueue *Queue) Close(operationContext context.Context) error {
	select {
	case <-messageQueue.done:
		return nil
	default:
	}
	_, operationError := messageQueue.ask(operationContext, queueCommand{kind: closeQueue})
	if operationError == ErrQueueClosed {
		return nil
	}
	return operationError
}

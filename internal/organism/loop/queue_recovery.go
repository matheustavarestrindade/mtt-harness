package loop

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (messageQueue *Queue) restoreSessions(operationContext context.Context, command queueCommand) {
	completed := directoryCompletion{command: command}
	completed.restored, completed.operationError = messageQueue.readRecovery(operationContext)
	messageQueue.completed <- completed
}

func (messageQueue *Queue) readRecovery(operationContext context.Context) ([]restoredSession, error) {
	entries, operationError := messageQueue.loop.configuration.Store.Queue().All(operationContext)
	if operationError != nil {
		return nil, operationError
	}
	var restored []restoredSession
	positions := map[atom.SessionID]int{}
	for _, entry := range entries {
		session, operationError := messageQueue.loop.configuration.Store.Sessions().Get(operationContext, entry.Message.SessionID)
		if operationError != nil {
			return nil, operationError
		}
		if entry.Running {
			if operationError := messageQueue.loop.repairToolHistory(operationContext, session); operationError != nil {
				return nil, operationError
			}
			if operationError := messageQueue.loop.emit(operationContext, session, atom.EventName("run.interrupted"), map[string]any{"message_id": entry.Message.ID}); operationError != nil {
				return nil, operationError
			}
			if operationError := messageQueue.loop.configuration.Store.Queue().Finish(operationContext, entry.Message.ID); operationError != nil {
				return nil, operationError
			}
			continue
		}
		position, found := positions[session.ID]
		if !found {
			position = len(restored)
			positions[session.ID] = position
			restored = append(restored, restoredSession{session: session})
		}
		restored[position].pending = append(restored[position].pending, entry.Message)
	}
	return restored, nil
}

func (messageQueue *Queue) controlSessions(command queueCommand, coordinators []*sessionCoordinator) {
	// Admission changes are committed by the directory. Once accepted, caller
	// cancellation only abandons its reply; it cannot undo a stop or resume.
	operationContext := context.WithoutCancel(command.operationContext)
	kind := stopSession
	if command.kind == resumeInstance {
		kind = resumeSession
	}
	if command.kind == closeQueue {
		kind = closeSession
	}
	results := make(chan error, len(coordinators))
	for _, coordinator := range coordinators {
		go func() {
			_, operationError := coordinator.ask(operationContext, sessionCommand{kind: kind})
			if kind == closeSession {
				<-coordinator.done
			}
			results <- operationError
		}()
	}
	var operationError error
	if command.kind == stopInstance {
		operationError = messageQueue.loop.cancelInstance(operationContext, command.instanceID)
	}
	for range coordinators {
		operationError = errors.Join(operationError, <-results)
	}
	messageQueue.completed <- directoryCompletion{command: command, operationError: operationError}
}

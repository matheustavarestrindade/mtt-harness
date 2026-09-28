package loop

import (
	"context"
	"fmt"
)

func (coordinator *sessionCoordinator) startOperation(command sessionCommand) {
	operationContext, cancel := context.WithCancel(command.operationContext)
	coordinator.working = &sessionOperation{command: command, cancel: cancel}
	active := coordinator.active
	go func() {
		defer cancel()
		completed := operationCompletion{command: command}
		switch command.kind {
		case submitMessage:
			completed.response.operationError = coordinator.loop.configuration.Store.Queue().Enqueue(operationContext, command.message, pendingMessageLimit)
		case cancelPending:
			completed.response.cancelled, completed.response.operationError = coordinator.loop.configuration.Store.Queue().Remove(operationContext, coordinator.session.ID, command.messageID)
		case revertConversation:
			completed = coordinator.revertHistory(operationContext, command, active)
		default:
			panic("unknown session operation")
		}
		// There is one I/O worker and one completion slot. The owner cannot exit
		// until it consumes this result, even if the requesting client leaves.
		coordinator.completed <- completed
	}()
}

func (coordinator *sessionCoordinator) handleOperation(completed operationCompletion) {
	coordinator.working.cancel()
	coordinator.working = nil
	response := completed.response
	switch completed.command.kind {
	case submitMessage:
		if response.operationError == nil {
			coordinator.pending = append(coordinator.pending, completed.command.message)
			response.position = len(coordinator.pending)
		}
	case cancelPending:
		if response.cancelled {
			for index, message := range coordinator.pending {
				if message.ID == completed.command.messageID {
					coordinator.pending = append(coordinator.pending[:index], coordinator.pending[index+1:]...)
					break
				}
			}
		}
	case revertConversation:
		coordinator.revert = nil
		if completed.pendingCleared {
			coordinator.pending = nil
		}
		if coordinator.mode == sessionReverting {
			coordinator.mode = sessionAccepting
			if completed.storageChanged && response.operationError != nil {
				coordinator.mode = sessionFailed
			}
		}
		if response.operationError == nil {
			coordinator.lastError = ""
		}
	}
	completed.command.respond(response)
}

func (coordinator *sessionCoordinator) revertHistory(operationContext context.Context, command sessionCommand, active *runningTurn) operationCompletion {
	completed := operationCompletion{command: command}
	completed.response.operationError = func() error {
		messages, operationError := coordinator.loop.configuration.Store.Sessions().Messages(operationContext, coordinator.session.ID)
		if operationError != nil {
			return operationError
		}
		found := false
		for _, message := range messages {
			if message.ID == command.messageID {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("message is not in the session")
		}
		if active != nil {
			active.cancel()
		}
		if operationError := coordinator.loop.cancelAndWait(operationContext, coordinator.session.ID); operationError != nil {
			return operationError
		}
		if active != nil {
			select {
			case <-active.done:
			case <-operationContext.Done():
				return operationContext.Err()
			}
		}
		completed.storageChanged = true
		if _, operationError := coordinator.loop.configuration.Store.Queue().ClearPending(operationContext, coordinator.session.ID); operationError != nil {
			return operationError
		}
		completed.pendingCleared = true
		removed, operationError := coordinator.loop.configuration.Store.Sessions().DeleteAfter(operationContext, coordinator.session.ID, command.messageID)
		if operationError != nil {
			return operationError
		}
		completed.response.removed = removed
		coordinator.loop.mutex.Lock()
		delete(coordinator.loop.groups, coordinator.session.ID)
		delete(coordinator.loop.finished, coordinator.session.ID)
		coordinator.loop.mutex.Unlock()
		return nil
	}()
	return completed
}

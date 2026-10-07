package loop

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

// DeleteConversation removes an idle session tree. Its worker fences the tree
// before deletion; caller cancellation abandons only the wait for the worker.
func (messageQueue *Queue) DeleteConversation(operationContext context.Context, session atom.Session) ([]atom.SessionID, error) {
	response, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{
		kind: deleteConversation, session: session, instanceID: session.InstanceID,
	})
	return response.deleted, operationError
}

func (messageQueue *Queue) deleteStoredConversation(command queueCommand) {
	operationContext := context.WithoutCancel(command.operationContext)
	completed := directoryCompletion{command: command}
	completed.operationError = func() (operationError error) {
		sessions, operationError := messageQueue.loop.configuration.Store.Sessions().List(operationContext, command.instanceID)
		if operationError != nil {
			return operationError
		}
		selected, protected, operationError := selectConversationSessions(sessions, command.session.ID)
		if operationError != nil {
			return operationError
		}
		// Install the narrow admission fence only after the worker knows the tree.
		// This also captures coordinators created while the storage read was pending.
		completed.protected = protected
		response, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{
			kind: fenceConversationDeletion, instanceID: command.instanceID, protected: protected,
		})
		if operationError != nil {
			return operationError
		}
		var frozen []*sessionCoordinator
		defer func() {
			for _, coordinator := range frozen {
				_, releaseError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: releaseSessionDeletion})
				if releaseError != nil && !errors.Is(releaseError, ErrQueueClosed) {
					operationError = errors.Join(operationError, releaseError)
				}
			}
		}()
		for _, coordinator := range response.coordinators {
			if !protected[coordinator.session.ID] {
				continue
			}
			if _, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: prepareSessionDeletion}); operationError != nil {
				if errors.Is(operationError, ErrQueueClosed) {
					<-coordinator.done
					continue
				}
				return operationError
			}
			frozen = append(frozen, coordinator)
		}
		if operationError := messageQueue.loop.fenceConversationRuns(protected); operationError != nil {
			return operationError
		}
		defer func() { messageQueue.loop.finishConversationDeletion(protected, completed.deleted) }()
		for identifier := range selected {
			processes, operationError := messageQueue.loop.configuration.Store.Processes().List(operationContext, identifier)
			if operationError != nil {
				return operationError
			}
			for _, process := range processes {
				if process.Status == "running" {
					return store.ErrConversationBusy
				}
			}
		}
		identifiers := make([]atom.SessionID, 0, len(selected))
		for identifier := range selected {
			identifiers = append(identifiers, identifier)
		}
		completeHistory, operationError := messageQueue.loop.beginPluginHistoryChange(operationContext, command.instanceID, harness.HistoryDelete, identifiers, "")
		if operationError != nil {
			return operationError
		}
		historyCommitted := false
		defer func() {
			operationError = errors.Join(operationError, completeHistory(context.WithoutCancel(operationContext), historyCommitted))
		}()
		completed.deleted, operationError = messageQueue.loop.configuration.Store.Sessions().DeleteConversation(operationContext, command.session.ID)
		if operationError != nil {
			return operationError
		}
		historyCommitted = true
		for _, coordinator := range frozen {
			if !selected[coordinator.session.ID] {
				continue
			}
			_, closeError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: closeSession})
			if closeError != nil && !errors.Is(closeError, ErrQueueClosed) {
				return closeError
			}
			<-coordinator.done
		}
		return nil
	}()
	messageQueue.completed <- completed
}

// Ancestors are protected while deleting a child: an active parent can still
// consume that child's completion result after the child run itself ends.
func selectConversationSessions(sessions []atom.Session, root atom.SessionID) (map[atom.SessionID]bool, map[atom.SessionID]bool, error) {
	byID := make(map[atom.SessionID]atom.Session, len(sessions))
	children := make(map[atom.SessionID][]atom.SessionID)
	for _, session := range sessions {
		byID[session.ID] = session
		children[session.Parent] = append(children[session.Parent], session.ID)
	}
	if _, found := byID[root]; !found {
		return nil, nil, store.ErrSessionNotFound
	}
	selected := map[atom.SessionID]bool{}
	pending := []atom.SessionID{root}
	for len(pending) > 0 {
		identifier := pending[0]
		pending = pending[1:]
		if selected[identifier] {
			continue
		}
		selected[identifier] = true
		pending = append(pending, children[identifier]...)
	}
	protected := make(map[atom.SessionID]bool, len(selected))
	for identifier := range selected {
		protected[identifier] = true
	}
	for ancestor := byID[root].Parent; ancestor != "" && !protected[ancestor]; ancestor = byID[ancestor].Parent {
		if _, found := byID[ancestor]; !found {
			break
		}
		protected[ancestor] = true
	}
	return selected, protected, nil
}

func (agentLoop *Loop) fenceConversationRuns(identifiers map[atom.SessionID]bool) error {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	for identifier := range identifiers {
		if agentLoop.activeRuns[identifier] != nil || agentLoop.sessionBlocks[identifier] != nil {
			return store.ErrConversationBusy
		}
	}
	for identifier := range identifiers {
		agentLoop.sessionBlocks[identifier] = store.ErrConversationBusy
	}
	return nil
}

func (agentLoop *Loop) finishConversationDeletion(protected map[atom.SessionID]bool, deleted []atom.SessionID) {
	agentLoop.mutex.Lock()
	defer agentLoop.mutex.Unlock()
	for identifier := range protected {
		delete(agentLoop.sessionBlocks, identifier)
	}
	for _, identifier := range deleted {
		agentLoop.sessionBlocks[identifier] = store.ErrSessionDeleted
		delete(agentLoop.groups, identifier)
		delete(agentLoop.finished, identifier)
	}
}

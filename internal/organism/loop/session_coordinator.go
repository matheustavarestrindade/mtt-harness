package loop

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
)

type sessionMode uint8

const (
	sessionAccepting sessionMode = iota
	sessionReverting
	sessionStopped
	sessionFailed
	sessionClosing
)

// run exclusively owns the mutable state below. I/O workers receive value
// copies and report completion; they never mutate the coordinator's fields.
type sessionCoordinator struct {
	loop        *Loop
	session     atom.Session
	commands    chan sessionCommand
	completed   chan operationCompletion
	runFinished chan runCompletion
	done        chan struct{}
	mode        sessionMode
	pending     []atom.Message
	mutations   []sessionCommand
	working     *sessionOperation
	active      *runningTurn
	revert      *sessionCommand
	waiters     []sessionCommand
	lastError   string
}

func newSessionCoordinator(agentLoop *Loop, session atom.Session, pending []atom.Message, stopped bool) *sessionCoordinator {
	coordinator := &sessionCoordinator{loop: agentLoop, session: session, pending: pending, commands: make(chan sessionCommand, commandBufferSize), completed: make(chan operationCompletion, 1), runFinished: make(chan runCompletion, 1), done: make(chan struct{})}
	if stopped {
		coordinator.mode = sessionStopped
	}
	go coordinator.run()
	return coordinator
}

func (coordinator *sessionCoordinator) run() {
	defer close(coordinator.done)
	for {
		if !coordinator.advance() {
			return
		}
		select {
		case command := <-coordinator.commands:
			coordinator.handle(command)
		case completed := <-coordinator.completed:
			coordinator.handleOperation(completed)
		case completed := <-coordinator.runFinished:
			coordinator.handleRunFinished(completed)
		}
	}
}

func (coordinator *sessionCoordinator) handle(command sessionCommand) {
	if operationError := command.operationContext.Err(); operationError != nil && command.kind != closeSession {
		command.respond(sessionReply{operationError: operationError})
		return
	}
	switch command.kind {
	case readStatus:
		identifiers := make([]string, 0, len(coordinator.pending))
		for _, message := range coordinator.pending {
			identifiers = append(identifiers, message.ID)
		}
		command.respond(sessionReply{status: QueueStatus{Running: coordinator.active != nil, Messages: identifiers, Error: coordinator.lastError}})
	case cancelCurrent:
		cancelled := coordinator.loop.CancelRun(coordinator.session.ID)
		if coordinator.active != nil {
			coordinator.active.cancel()
			cancelled = true
		}
		command.respond(sessionReply{cancelled: cancelled})
	case submitMessage, cancelPending:
		coordinator.handleMutation(command)
	case revertConversation:
		coordinator.handleRevert(command)
	case stopSession, closeSession:
		coordinator.handleStop(command)
	case resumeSession:
		if coordinator.mode == sessionClosing {
			command.respond(sessionReply{operationError: ErrQueueClosed})
			return
		}
		if coordinator.mode == sessionReverting || len(coordinator.waiters) > 0 {
			command.respond(sessionReply{operationError: ErrSessionBusy})
			return
		}
		coordinator.mode = sessionAccepting
		command.respond(sessionReply{})
	default:
		panic("unknown session command")
	}
}

func (coordinator *sessionCoordinator) handleMutation(command sessionCommand) {
	if coordinator.mode == sessionClosing {
		command.respond(sessionReply{operationError: ErrQueueClosed})
		return
	}
	if coordinator.mode == sessionReverting || coordinator.mode == sessionFailed {
		command.respond(sessionReply{operationError: ErrSessionBusy})
		return
	}
	if command.kind == submitMessage {
		if coordinator.mode == sessionStopped || !coordinator.instanceRunning() {
			command.respond(sessionReply{operationError: ErrInstanceStopped})
			return
		}
		waiting := len(coordinator.pending)
		for _, mutation := range coordinator.mutations {
			if mutation.kind == submitMessage {
				waiting++
			}
		}
		if coordinator.working != nil && coordinator.working.command.kind == submitMessage {
			waiting++
		}
		if waiting >= pendingMessageLimit {
			command.respond(sessionReply{operationError: store.ErrQueueFull})
			return
		}
	}
	if len(coordinator.mutations) >= pendingMessageLimit {
		command.respond(sessionReply{operationError: ErrSessionBusy})
		return
	}
	if command.kind == cancelPending && coordinator.active != nil && command.messageID == coordinator.active.messageID {
		command.respond(sessionReply{})
		return
	}
	coordinator.mutations = append(coordinator.mutations, command)
}

func (coordinator *sessionCoordinator) handleRevert(command sessionCommand) {
	if coordinator.mode == sessionClosing {
		command.respond(sessionReply{operationError: ErrQueueClosed})
		return
	}
	if coordinator.mode == sessionReverting || coordinator.mode == sessionStopped {
		command.respond(sessionReply{operationError: ErrSessionBusy})
		return
	}
	coordinator.mode = sessionReverting
	coordinator.revert = &command
	coordinator.rejectMutations(ErrSessionBusy)
}

func (coordinator *sessionCoordinator) handleStop(command sessionCommand) {
	if command.kind == closeSession {
		coordinator.mode = sessionClosing
	}
	if coordinator.mode != sessionClosing {
		coordinator.mode = sessionStopped
	}
	if coordinator.active != nil {
		coordinator.active.cancel()
	}
	if coordinator.working != nil && (command.kind == closeSession || coordinator.working.command.kind == revertConversation) {
		coordinator.working.cancel()
	}
	if coordinator.revert != nil && (coordinator.working == nil || coordinator.working.command.kind != revertConversation) {
		coordinator.revert.respond(sessionReply{operationError: ErrSessionBusy})
		coordinator.revert = nil
	}
	coordinator.rejectMutations(ErrSessionBusy)
	coordinator.waiters = append(coordinator.waiters, command)
}

func (coordinator *sessionCoordinator) rejectMutations(operationError error) {
	for _, command := range coordinator.mutations {
		command.respond(sessionReply{operationError: operationError})
	}
	coordinator.mutations = nil
}

func (coordinator *sessionCoordinator) advance() bool {
	if coordinator.working == nil {
		if coordinator.revert != nil {
			coordinator.startOperation(*coordinator.revert)
		}
		for coordinator.working == nil && len(coordinator.mutations) > 0 {
			command := coordinator.mutations[0]
			coordinator.mutations = coordinator.mutations[1:]
			if operationError := command.operationContext.Err(); operationError != nil {
				command.respond(sessionReply{operationError: operationError})
				continue
			}
			coordinator.startOperation(command)
		}
	}
	if coordinator.mode == sessionAccepting && coordinator.active == nil && coordinator.working == nil && len(coordinator.pending) > 0 {
		if !coordinator.instanceRunning() {
			coordinator.mode = sessionStopped
		} else {
			coordinator.startRun()
		}
	}
	if coordinator.active != nil || coordinator.working != nil || coordinator.revert != nil {
		return true
	}
	for _, waiter := range coordinator.waiters {
		waiter.respond(sessionReply{})
	}
	coordinator.waiters = nil
	return coordinator.mode != sessionClosing
}

func (coordinator *sessionCoordinator) instanceRunning() bool {
	return coordinator.loop.configuration.Instances == nil || coordinator.loop.configuration.Instances.IsRunning(coordinator.session.InstanceID)
}

func (coordinator *sessionCoordinator) handleRunFinished(completed runCompletion) {
	coordinator.active = nil
	if completed.operationError != nil && !errors.Is(completed.operationError, context.Canceled) {
		coordinator.lastError = completed.operationError.Error()
	}
	if completed.finishError != nil {
		coordinator.lastError = completed.finishError.Error()
		if coordinator.mode == sessionAccepting {
			coordinator.mode = sessionFailed
		}
	}
}

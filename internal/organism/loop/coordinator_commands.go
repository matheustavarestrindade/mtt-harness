package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type sessionCommandKind uint8

const (
	submitMessage sessionCommandKind = iota
	cancelCurrent
	cancelPending
	readStatus
	revertConversation
	stopSession
	resumeSession
	closeSession
)

// A command gets exactly one reply. The single-slot reply channel lets a
// caller cancel its wait without blocking the owner that completes the work.
type sessionCommand struct {
	kind             sessionCommandKind
	operationContext context.Context
	message          atom.Message
	messageID        string
	reply            chan sessionReply
}

type sessionReply struct {
	position       int
	cancelled      bool
	removed        int
	status         QueueStatus
	operationError error
}

type sessionOperation struct {
	command sessionCommand
	cancel  context.CancelFunc
}

type operationCompletion struct {
	command        sessionCommand
	response       sessionReply
	pendingCleared bool
	storageChanged bool
}

type runCompletion struct {
	operationError error
	finishError    error
}

type runningTurn struct {
	messageID string
	cancel    context.CancelFunc
	done      chan struct{}
}

func (coordinator *sessionCoordinator) requestSessionCommand(operationContext context.Context, command sessionCommand) (sessionReply, error) {
	command.operationContext = operationContext
	command.reply = make(chan sessionReply, 1)
	if operationError := operationContext.Err(); operationError != nil {
		return sessionReply{}, operationError
	}
	select {
	case <-coordinator.done:
		return sessionReply{}, ErrQueueClosed
	default:
	}
	select {
	case <-operationContext.Done():
		return sessionReply{}, operationContext.Err()
	case <-coordinator.done:
		return sessionReply{}, ErrQueueClosed
	case coordinator.commands <- command:
	}
	select {
	case response := <-command.reply:
		return response, response.operationError
	case <-operationContext.Done():
		return sessionReply{}, operationContext.Err()
	case <-coordinator.done:
		select {
		case response := <-command.reply:
			return response, response.operationError
		default:
			return sessionReply{}, ErrQueueClosed
		}
	}
}

func (command sessionCommand) respond(response sessionReply) {
	command.reply <- response
}

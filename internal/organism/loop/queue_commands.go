package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type queueCommandKind uint8

const (
	findSession queueCommandKind = iota
	restoreQueue
	stopInstance
	resumeInstance
	closeQueue
	deleteConversation
	fenceConversationDeletion
)

type queueCommand struct {
	kind             queueCommandKind
	operationContext context.Context
	session          atom.Session
	create           bool
	instanceID       string
	protected        map[atom.SessionID]bool
	reply            chan queueReply
}

type queueReply struct {
	coordinator    *sessionCoordinator
	coordinators   []*sessionCoordinator
	deleted        []atom.SessionID
	operationError error
}

type restoredSession struct {
	session atom.Session
	pending []atom.Message
}

type directoryCompletion struct {
	command        queueCommand
	restored       []restoredSession
	deleted        []atom.SessionID
	protected      map[atom.SessionID]bool
	operationError error
}

func (messageQueue *Queue) coordinator(operationContext context.Context, session atom.Session, create bool) (*sessionCoordinator, error) {
	response, operationError := messageQueue.requestDirectoryCommand(operationContext, queueCommand{kind: findSession, session: session, create: create})
	return response.coordinator, operationError
}

func (messageQueue *Queue) requestDirectoryCommand(operationContext context.Context, command queueCommand) (queueReply, error) {
	command.operationContext = operationContext
	command.reply = make(chan queueReply, 1)
	if operationError := operationContext.Err(); operationError != nil {
		return queueReply{}, operationError
	}
	select {
	case <-messageQueue.done:
		return queueReply{}, ErrQueueClosed
	default:
	}
	select {
	case <-operationContext.Done():
		return queueReply{}, operationContext.Err()
	case <-messageQueue.done:
		return queueReply{}, ErrQueueClosed
	case messageQueue.commands <- command:
	}
	select {
	case response := <-command.reply:
		return response, response.operationError
	case <-operationContext.Done():
		return queueReply{}, operationContext.Err()
	case <-messageQueue.done:
		select {
		case response := <-command.reply:
			return response, response.operationError
		default:
			return queueReply{}, ErrQueueClosed
		}
	}
}

func (command queueCommand) respond(response queueReply) {
	command.reply <- response
}

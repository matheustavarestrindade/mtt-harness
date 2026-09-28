package loop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var ErrSessionBusy = errors.New("session is being reverted or stopped")
var ErrInstanceStopped = errors.New("instance is stopped")
var ErrQueueClosed = errors.New("message queue is closed")
var ErrInvalidContent = errors.New("invalid message content")

const pendingMessageLimit = 128
const commandBufferSize = 32

// Queue is a request/reply facade. Its directory and each session have one
// owning goroutine. Neither owner executes database operations or model work.
// Close joins their workers before the caller closes the database.
type Queue struct {
	loop      *Loop
	commands  chan queueCommand
	completed chan directoryCompletion
	done      chan struct{}
}

// QueueStatus is a detached snapshot, including any terminal operation error.
type QueueStatus struct {
	Running  bool
	Messages []string
	Error    string
}

func NewQueue(agentLoop *Loop) *Queue {
	messageQueue := &Queue{loop: agentLoop, commands: make(chan queueCommand, commandBufferSize), completed: make(chan directoryCompletion, commandBufferSize), done: make(chan struct{})}
	go messageQueue.runDirectory()
	return messageQueue
}

func (messageQueue *Queue) Submit(operationContext context.Context, session atom.Session, content string) (atom.Message, int, error) {
	return messageQueue.SubmitContent(operationContext, session, []atom.Content{{Type: atom.Text, Text: content}})
}

func (messageQueue *Queue) SubmitContent(operationContext context.Context, session atom.Session, content []atom.Content) (atom.Message, int, error) {
	if operationError := validateMessageContent(content); operationError != nil {
		return atom.Message{}, 0, operationError
	}
	// Ownership transfers through the inbox. Callers cannot mutate queued media.
	ownedContent := append([]atom.Content(nil), content...)
	for index := range ownedContent {
		ownedContent[index].Data = append([]byte(nil), content[index].Data...)
	}
	message := atom.Message{ID: newID(), SessionID: session.ID, Role: atom.RoleUser, Content: ownedContent, CreatedAt: time.Now()}
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return atom.Message{}, 0, operationError
	}
	response, operationError := coordinator.ask(operationContext, sessionCommand{kind: submitMessage, message: message})
	message.Content = append([]atom.Content(nil), message.Content...)
	for index := range message.Content {
		message.Content[index].Data = append([]byte(nil), message.Content[index].Data...)
	}
	return message, response.position, operationError
}

func (messageQueue *Queue) Cancel(operationContext context.Context, sessionID atom.SessionID) (bool, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, atom.Session{ID: sessionID}, false)
	if operationError != nil {
		return false, operationError
	}
	if coordinator == nil {
		return messageQueue.loop.CancelRun(sessionID), nil
	}
	response, operationError := coordinator.ask(operationContext, sessionCommand{kind: cancelCurrent})
	return response.cancelled, operationError
}

func (messageQueue *Queue) CancelMessage(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error) {
	session, operationError := messageQueue.loop.configuration.Store.Sessions().Get(operationContext, sessionID)
	if operationError != nil {
		return false, operationError
	}
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return false, operationError
	}
	response, operationError := coordinator.ask(operationContext, sessionCommand{kind: cancelPending, messageID: messageID})
	return response.cancelled, operationError
}

func (messageQueue *Queue) Status(operationContext context.Context, sessionID atom.SessionID) (QueueStatus, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, atom.Session{ID: sessionID}, false)
	if operationError != nil || coordinator == nil {
		return QueueStatus{}, operationError
	}
	response, operationError := coordinator.ask(operationContext, sessionCommand{kind: readStatus})
	return response.status, operationError
}

func validateMessageContent(content []atom.Content) error {
	if len(content) == 0 {
		return fmt.Errorf("%w: content is required", ErrInvalidContent)
	}
	for _, item := range content {
		switch item.Type {
		case atom.Text:
		case atom.Image, atom.Audio, atom.File:
			if len(item.Data) == 0 && item.URL == "" {
				return fmt.Errorf("%w: media requires data or a URL", ErrInvalidContent)
			}
		default:
			return fmt.Errorf("%w: unknown media type %q", ErrInvalidContent, item.Type)
		}
	}
	return nil
}

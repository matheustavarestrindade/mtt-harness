package loop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

var ErrSessionBusy = errors.New("session is being reverted or stopped")
var ErrInstanceStopped = errors.New("instance is stopped")
var ErrQueueClosed = errors.New("message queue is closed")
var ErrInvalidContent = errors.New("invalid message content")

const pendingMessageLimit = 128

// Queue serializes turns per session. Its lock also fences enqueue, cancel and
// revert; generation never runs under that lock. Persistence precedes the 202.
type Queue struct {
	loop        *Loop
	mutex       sync.Mutex
	workers     map[atom.SessionID]*sessionWorker
	closed      bool
	restoring   bool
	workersDone sync.WaitGroup
}

type sessionWorker struct {
	session    atom.Session
	queue      []atom.Message
	running    bool
	paused     bool
	cancel     context.CancelFunc
	activeDone chan struct{}
	lastError  string
}

func NewQueue(agentLoop *Loop) *Queue {
	return &Queue{loop: agentLoop, workers: map[atom.SessionID]*sessionWorker{}}
}

func (messageQueue *Queue) Submit(operationContext context.Context, session atom.Session, content string) (atom.Message, int, error) {
	return messageQueue.SubmitContent(operationContext, session, []atom.Content{{Type: atom.Text, Text: content}})
}

func (messageQueue *Queue) SubmitContent(operationContext context.Context, session atom.Session, content []atom.Content) (atom.Message, int, error) {
	if len(content) == 0 {
		return atom.Message{}, 0, fmt.Errorf("%w: content is required", ErrInvalidContent)
	}
	for _, item := range content {
		switch item.Type {
		case atom.Text:
		case atom.Image, atom.Audio, atom.File:
			if len(item.Data) == 0 && item.URL == "" {
				return atom.Message{}, 0, fmt.Errorf("%w: media requires data or a URL", ErrInvalidContent)
			}
		default:
			return atom.Message{}, 0, fmt.Errorf("%w: unknown media type %q", ErrInvalidContent, item.Type)
		}
	}
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	if messageQueue.closed {
		return atom.Message{}, 0, ErrQueueClosed
	}
	if messageQueue.restoring {
		return atom.Message{}, 0, ErrSessionBusy
	}
	if messageQueue.loop.configuration.Instances != nil && !messageQueue.loop.configuration.Instances.IsRunning(session.InstanceID) {
		return atom.Message{}, 0, ErrInstanceStopped
	}
	worker := messageQueue.worker(session)
	if worker.paused {
		return atom.Message{}, 0, ErrSessionBusy
	}
	message := atom.Message{ID: newID(), SessionID: session.ID, Role: atom.RoleUser, Content: content, CreatedAt: time.Now()}
	if operationError := messageQueue.loop.configuration.Store.Queue().Enqueue(operationContext, message, pendingMessageLimit); operationError != nil {
		return atom.Message{}, 0, operationError
	}
	worker.queue = append(worker.queue, message)
	position := len(worker.queue)
	messageQueue.startWorker(worker)
	return message, position, nil
}

// worker and startWorker require the queue lock.
func (messageQueue *Queue) worker(session atom.Session) *sessionWorker {
	worker := messageQueue.workers[session.ID]
	if worker == nil {
		worker = &sessionWorker{session: session}
		messageQueue.workers[session.ID] = worker
	}
	return worker
}
func (messageQueue *Queue) startWorker(worker *sessionWorker) {
	if messageQueue.closed || worker.running || worker.paused || len(worker.queue) == 0 {
		return
	}
	if messageQueue.loop.configuration.Instances != nil && !messageQueue.loop.configuration.Instances.IsRunning(worker.session.InstanceID) {
		worker.paused = true
		return
	}
	worker.running = true
	messageQueue.workersDone.Add(1)
	go messageQueue.work(worker)
}

func (messageQueue *Queue) work(worker *sessionWorker) {
	defer messageQueue.workersDone.Done()
	for {
		messageQueue.mutex.Lock()
		if messageQueue.closed || worker.paused || len(worker.queue) == 0 {
			worker.running = false
			if !worker.paused && len(worker.queue) == 0 && worker.lastError == "" {
				delete(messageQueue.workers, worker.session.ID)
			}
			messageQueue.mutex.Unlock()
			return
		}
		message := worker.queue[0]
		worker.queue = worker.queue[1:]
		operationContext, cancel := context.WithCancel(context.Background())
		worker.cancel, worker.activeDone = cancel, make(chan struct{})
		messageQueue.mutex.Unlock()
		operationError := messageQueue.loop.runSession(operationContext, worker.session, message.ID)
		cancel()
		completionContext, finish := context.WithTimeout(context.Background(), 5*time.Second)
		finishError := messageQueue.loop.configuration.Store.Queue().Finish(completionContext, message.ID)
		if operationError != nil {
			status := "error"
			if errors.Is(operationError, context.Canceled) {
				status = "cancelled"
			}
			if eventError := messageQueue.loop.emit(completionContext, worker.session, atom.EventName("run."+status), map[string]any{"message_id": message.ID, "error": operationError.Error()}); eventError != nil {
				log.Printf("mtt: record run outcome: %v", eventError)
			}
		}
		finish()
		messageQueue.mutex.Lock()
		worker.cancel = nil
		if operationError != nil && !errors.Is(operationError, context.Canceled) {
			worker.lastError = operationError.Error()
		}
		if finishError != nil {
			worker.lastError = finishError.Error()
			worker.paused = true
		}
		close(worker.activeDone)
		worker.activeDone = nil
		messageQueue.mutex.Unlock()
	}
}

func (messageQueue *Queue) Cancel(sessionID atom.SessionID) bool {
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	worker := messageQueue.workers[sessionID]
	if worker == nil || worker.cancel == nil {
		return messageQueue.loop.CancelRun(sessionID)
	}
	worker.cancel()
	return true
}

func (messageQueue *Queue) CancelMessage(operationContext context.Context, sessionID atom.SessionID, messageID string) (bool, error) {
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	removed, operationError := messageQueue.loop.configuration.Store.Queue().Remove(operationContext, sessionID, messageID)
	if operationError != nil || !removed {
		return removed, operationError
	}
	if worker := messageQueue.workers[sessionID]; worker != nil {
		for index, message := range worker.queue {
			if message.ID == messageID {
				worker.queue = append(worker.queue[:index], worker.queue[index+1:]...)
				break
			}
		}
	}
	return true, nil
}

func (messageQueue *Queue) Status(sessionID atom.SessionID) (bool, []string) {
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	worker := messageQueue.workers[sessionID]
	if worker == nil {
		return false, nil
	}
	identifiers := make([]string, 0, len(worker.queue))
	for _, message := range worker.queue {
		identifiers = append(identifiers, message.ID)
	}
	return worker.cancel != nil, identifiers
}

func (messageQueue *Queue) LastError(sessionID atom.SessionID) string {
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	if worker := messageQueue.workers[sessionID]; worker != nil {
		return worker.lastError
	}
	return ""
}

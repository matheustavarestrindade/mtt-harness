package loop

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

// Revert pauses admission, waits for the old writer, and only then removes
// history. Holding the pause through deletion prevents a new turn slipping in.
func (messageQueue *Queue) Revert(operationContext context.Context, session atom.Session, messageID string) (int, error) {
	messageQueue.mutex.Lock()
	worker := messageQueue.worker(session)
	if worker.paused {
		messageQueue.mutex.Unlock()
		return 0, ErrSessionBusy
	}
	worker.paused = true
	messageQueue.mutex.Unlock()
	defer func() {
		messageQueue.mutex.Lock()
		defer messageQueue.mutex.Unlock()
		worker.paused = false
		messageQueue.startWorker(worker)
	}()
	messages, operationError := messageQueue.loop.configuration.Store.Sessions().Messages(operationContext, session.ID)
	if operationError != nil {
		return 0, operationError
	}
	found := false
	for _, message := range messages {
		if message.ID == messageID {
			found = true
			break
		}
	}
	if !found {
		return 0, fmt.Errorf("message is not in the session")
	}
	messageQueue.mutex.Lock()
	_, operationError = messageQueue.loop.configuration.Store.Queue().ClearPending(operationContext, session.ID)
	if operationError != nil {
		messageQueue.mutex.Unlock()
		return 0, operationError
	}
	worker.queue = nil
	if worker.cancel != nil {
		worker.cancel()
	}
	done := worker.activeDone
	messageQueue.mutex.Unlock()
	if operationError := messageQueue.loop.cancelAndWait(operationContext, session.ID); operationError != nil {
		return 0, operationError
	}
	if done != nil {
		select {
		case <-done:
		case <-operationContext.Done():
			return 0, operationContext.Err()
		}
	}
	removed, operationError := messageQueue.loop.configuration.Store.Sessions().DeleteAfter(operationContext, session.ID, messageID)
	if operationError != nil {
		return 0, operationError
	}
	messageQueue.loop.mutex.Lock()
	delete(messageQueue.loop.groups, session.ID)
	delete(messageQueue.loop.finished, session.ID)
	messageQueue.loop.mutex.Unlock()
	return removed, nil
}

func (messageQueue *Queue) StopInstance(operationContext context.Context, instanceID string) error {
	messageQueue.mutex.Lock()
	var pending []chan struct{}
	for _, worker := range messageQueue.workers {
		if worker.session.InstanceID != instanceID {
			continue
		}
		worker.paused = true
		if worker.cancel != nil {
			worker.cancel()
			pending = append(pending, worker.activeDone)
		}
	}
	messageQueue.mutex.Unlock()
	if operationError := messageQueue.loop.cancelInstance(operationContext, instanceID); operationError != nil {
		return operationError
	}
	for _, done := range pending {
		select {
		case <-done:
		case <-operationContext.Done():
			return operationContext.Err()
		}
	}
	return nil
}

func (messageQueue *Queue) ResumeInstance(instanceID string) {
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	for _, worker := range messageQueue.workers {
		if worker.session.InstanceID == instanceID {
			worker.paused = false
			messageQueue.startWorker(worker)
		}
	}
}

// Restore keeps pending requests, but never retries an interrupted running
// request: external tools may already have performed irreversible actions.
func (messageQueue *Queue) Restore(operationContext context.Context) error {
	messageQueue.mutex.Lock()
	if messageQueue.closed || messageQueue.restoring || len(messageQueue.workers) > 0 {
		messageQueue.mutex.Unlock()
		return ErrSessionBusy
	}
	messageQueue.restoring = true
	messageQueue.mutex.Unlock()
	defer func() {
		messageQueue.mutex.Lock()
		messageQueue.restoring = false
		messageQueue.mutex.Unlock()
	}()
	entries, operationError := messageQueue.loop.configuration.Store.Queue().All(operationContext)
	if operationError != nil {
		return operationError
	}
	for _, entry := range entries {
		session, operationError := messageQueue.loop.configuration.Store.Sessions().Get(operationContext, entry.Message.SessionID)
		if operationError != nil {
			return operationError
		}
		if entry.Running {
			if operationError := messageQueue.loop.repairToolHistory(operationContext, session); operationError != nil {
				return operationError
			}
			if operationError := messageQueue.loop.emit(operationContext, session, atom.EventName("run.interrupted"), map[string]any{"message_id": entry.Message.ID}); operationError != nil {
				return operationError
			}
			if operationError := messageQueue.loop.configuration.Store.Queue().Finish(operationContext, entry.Message.ID); operationError != nil {
				return operationError
			}
			continue
		}
		messageQueue.mutex.Lock()
		worker := messageQueue.worker(session)
		worker.paused = messageQueue.loop.configuration.Instances != nil && !messageQueue.loop.configuration.Instances.IsRunning(session.InstanceID)
		worker.queue = append(worker.queue, entry.Message)
		messageQueue.mutex.Unlock()
	}
	messageQueue.mutex.Lock()
	defer messageQueue.mutex.Unlock()
	for _, worker := range messageQueue.workers {
		messageQueue.startWorker(worker)
	}
	return nil
}

func (messageQueue *Queue) Close(operationContext context.Context) error {
	messageQueue.mutex.Lock()
	messageQueue.closed = true
	for _, worker := range messageQueue.workers {
		worker.paused = true
		if worker.cancel != nil {
			worker.cancel()
		}
	}
	messageQueue.mutex.Unlock()
	done := make(chan struct{})
	go func() {
		messageQueue.workersDone.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-operationContext.Done():
		return operationContext.Err()
	}
}

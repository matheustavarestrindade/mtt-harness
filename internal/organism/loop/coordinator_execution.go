package loop

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (coordinator *sessionCoordinator) startRun() {
	message := coordinator.pending[0]
	coordinator.pending = coordinator.pending[1:]
	operationContext, cancel := context.WithCancel(context.Background())
	active := &runningTurn{messageID: message.ID, cancel: cancel, done: make(chan struct{})}
	coordinator.active = active
	coordinator.lastError = ""
	go func() {
		operationError := coordinator.loop.runSession(operationContext, coordinator.session, message.ID)
		completionContext, finish := context.WithTimeout(context.Background(), 5*time.Second)
		finishError := coordinator.loop.configuration.Store.Queue().Finish(completionContext, message.ID)
		if operationError != nil {
			status := "error"
			if errors.Is(operationError, context.Canceled) {
				status = "cancelled"
			}
			if eventError := coordinator.loop.emit(completionContext, coordinator.session, atom.EventName("run."+status), map[string]any{"message_id": message.ID, "error": operationError.Error()}); eventError != nil {
				log.Printf("mtt: record run outcome: %v", eventError)
			}
		}
		finish()
		cancel()
		close(active.done)
		coordinator.runFinished <- runCompletion{operationError: operationError, finishError: finishError}
	}()
}

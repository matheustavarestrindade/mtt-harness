package loop

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (coordinator *sessionCoordinator) startQueuedTurn() {
	message := coordinator.pending[0]
	coordinator.pending = coordinator.pending[1:]
	operationContext, cancel := context.WithCancel(context.Background())
	active := &runningTurn{messageID: message.ID, cancel: cancel, done: make(chan struct{})}
	coordinator.active = active
	coordinator.lastError = ""
	go func() {
		claimedIDs := []string{message.ID}
		var admissionFailure error
		admit := func(boundaryContext context.Context) ([]string, error) {
			identifiers, admissionError := coordinator.admitInput(boundaryContext)
			claimedIDs = append(claimedIDs, identifiers...)
			if errors.Is(admissionError, errQueuedAdmission) && !errors.Is(admissionError, context.Canceled) {
				admissionFailure = admissionError
			}
			if boundaryContext.Err() != nil {
				return nil, boundaryContext.Err()
			}
			return identifiers, admissionError
		}
		operationError := coordinator.loop.runSession(operationContext, coordinator.session, message.ID, admit)
		completionContext, finish := context.WithTimeout(context.Background(), 5*time.Second)
		finishError := admissionFailure
		for _, identifier := range claimedIDs {
			finishError = errors.Join(finishError, coordinator.loop.configuration.Store.Queue().Finish(completionContext, identifier))
		}
		if operationError != nil {
			status := "error"
			if errors.Is(operationError, context.Canceled) {
				status = "cancelled"
			}
			if eventError := coordinator.loop.emitSessionEvent(completionContext, coordinator.session, atom.EventName("run."+status), map[string]any{"message_id": message.ID, "message_ids": claimedIDs, "error": operationError.Error()}); eventError != nil {
				log.Printf("mtt: record run outcome: %v", eventError)
			}
		}
		finish()
		cancel()
		close(active.done)
		coordinator.runFinished <- runCompletion{operationError: operationError, finishError: finishError}
	}()
}

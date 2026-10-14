package loop

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/operation"
)

// runModelRound retains plugin request leases until every tool worker is joined,
// including error paths. Configuration cannot change halfway through a group.
func (agentLoop *Loop) runModelRound(session atom.Session, modelCall *preparedModelCall) (bool, error) {
	defer modelCall.release()
	operationContext := modelCall.operationContext
	roundContext, cancelRound := context.WithCancel(operationContext)
	defer cancelRound()
	if modelCall.commitReminders != nil {
		if operationError := modelCall.commitReminders(roundContext); operationError != nil {
			return false, operation.WrapError(operationError, "commit request reminders")
		}
	}
	message, tasks, operationError := agentLoop.receiveModelResponse(roundContext, session, modelCall)
	if operationError == nil {
		operationError = operation.WrapError(contextbuilder.CaptureReportedUsage(operationContext, modelCall.request, modelCall.provider, &message), "record request context usage")
	}
	if operationError != nil {
		cancelRound()
		results := waitForTools(tasks)
		// A provider interruption is not a completed response, but its partial
		// output and charges remain factual history. User cancellation/revert
		// keeps the existing writer fence and must not publish a late message.
		if message.ID != "" && operationContext.Err() == nil {
			saveError := agentLoop.saveModelResponse(operationContext, session, modelCall.modelID, message, "error")
			if saveError == nil && len(message.ToolCalls) > 0 {
				saveError = agentLoop.saveToolResults(operationContext, session, message.ToolCalls, results)
			}
			operationError = errors.Join(operationError, saveError)
		}
		return false, operation.WrapError(operationError, "receive model response")
	}
	if operationError := agentLoop.saveModelResponse(operationContext, session, modelCall.modelID, message, "completed"); operationError != nil {
		cancelRound()
		waitForTools(tasks)
		return false, operation.WrapError(operationError, "save model response")
	}
	if len(message.ToolCalls) == 0 {
		return true, agentLoop.recordTaskStateResponse(operationContext, session, modelCall)
	}
	results := waitForTools(tasks)
	cancelRound()
	if operationError := operationContext.Err(); operationError != nil {
		return false, operationError
	}
	if operationError := agentLoop.saveToolResults(operationContext, session, message.ToolCalls, results); operationError != nil {
		return false, operation.WrapError(operationError, "save tool results")
	}
	if operationError := agentLoop.recordTaskStateResponse(operationContext, session, modelCall); operationError != nil {
		return false, operation.WrapError(operationError, "record task state response")
	}
	return agentLoop.isFinished(session.ID), nil
}

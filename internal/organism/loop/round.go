package loop

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/internal/operation"
)

// runModelRound retains plugin request leases until every tool worker is joined,
// including error paths. Configuration cannot change halfway through a group.
func (agentLoop *Loop) runModelRound(session atom.Session, modelCall *preparedModelCall) (bool, error) {
	defer modelCall.release()
	operationContext := modelCall.operationContext
	roundContext, cancelRound := context.WithCancel(operationContext)
	defer cancelRound()
	message, tasks, operationError := agentLoop.receiveModelResponse(roundContext, session, modelCall)
	if operationError != nil {
		cancelRound()
		waitForTools(tasks)
		return false, operation.WrapError(operationError, "receive model response")
	}
	if operationError := agentLoop.saveModelResponse(operationContext, session, modelCall.modelID, message); operationError != nil {
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

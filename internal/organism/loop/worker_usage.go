package loop

import (
	"context"
	"errors"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (agentLoop *Loop) recordWorkerAttempt(operationContext context.Context, session atom.Session, model atom.ModelInfo, modelID, requestID string, started time.Time, reported *atom.Usage, responseError error) error {
	profile := session.Execution
	usage := atom.Usage{}
	if reported != nil {
		usage = *reported
		usage.Cost = calculateUsageCost(model, reported)
	}
	status := "completed"
	if responseError != nil {
		status = "error"
	}
	if errors.Is(responseError, context.Canceled) || operationContext.Err() != nil {
		status = "cancelled"
	}
	recordContext, cancel := context.WithTimeout(context.WithoutCancel(operationContext), 5*time.Second)
	defer cancel()
	operationError := agentLoop.configuration.Store.Usage().Save(recordContext, atom.UsageRecord{InstanceID: session.InstanceID, SourceSessionID: profile.SourceSessionID, Agent: profile.Agent, RunID: profile.RunID, RequestID: requestID, ModelID: modelID, Usage: usage, CreatedAt: started, Duration: time.Since(started), Status: status})
	agentLoop.notifyWorkspaceState(session.InstanceID)
	return operationError
}

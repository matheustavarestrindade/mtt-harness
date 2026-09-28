package loop

import (
	"context"
	"fmt"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func (agentLoop *Loop) allow(operationContext context.Context, session atom.Session, name string, verdict atom.Verdict) (bool, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return false, operationError
	}
	switch verdict.Kind {
	case atom.VerdictDeny:
		return false, nil
	case atom.VerdictAllow:
		return true, nil
	case atom.VerdictAsk:
	default:
		return false, fmt.Errorf("invalid permission verdict %q", verdict.Kind)
	}
	target := name
	if verdict.Target != "" {
		target += ":" + verdict.Target
	}
	decision, found, operationError := agentLoop.configuration.Engine.Cached(operationContext, session, target)
	if operationError != nil {
		return false, operationError
	}
	if found {
		return decision.Kind == atom.VerdictAllow, nil
	}
	request := atom.PermissionRequest{ID: newID(), InstanceID: session.InstanceID, SessionID: session.ID, Target: target, Why: verdict.Why}
	decision, operationError = agentLoop.configuration.Broker.Request(operationContext, request, func() error {
		return agentLoop.emit(operationContext, session, atom.EventPermissionRequest, request)
	})
	if operationError != nil {
		return false, operationError
	}
	if operationError := agentLoop.configuration.Engine.Remember(operationContext, session, target, decision); operationError != nil {
		return false, operationError
	}
	if operationError := agentLoop.emit(operationContext, session, atom.EventPermissionDecision, decision); operationError != nil {
		return false, operationError
	}
	return decision.Kind == atom.VerdictAllow, operationContext.Err()
}

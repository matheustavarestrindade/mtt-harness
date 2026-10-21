package loop

import (
	"context"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (messageQueue *Queue) reviewControl(operationContext context.Context, session atom.Session) (harness.SessionControlToken, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return harness.SessionControlToken{}, operationError
	}
	response, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: readControlToken})
	return response.control, operationError
}
func (messageQueue *Queue) resolveReviewPermission(operationContext context.Context, session atom.Session, control harness.SessionControlToken, decision atom.PermissionDecision) (bool, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return false, operationError
	}
	response, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: applyPermissionDecision, control: control, permission: decision})
	return response.applied, operationError
}
func (messageQueue *Queue) submitReviewContinuation(operationContext context.Context, session atom.Session, control harness.SessionControlToken, identifier, text string) (bool, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return false, operationError
	}
	message := atom.Message{ID: identifier, SessionID: session.ID, Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}
	response, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: submitFencedRuntime, control: control, message: message})
	return response.applied, operationError
}

func (messageQueue *Queue) submitPermissionCorrection(operationContext context.Context, session atom.Session, control harness.SessionControlToken, decision atom.PermissionDecision, text string) (bool, error) {
	coordinator, operationError := messageQueue.coordinator(operationContext, session, true)
	if operationError != nil {
		return false, operationError
	}
	message := atom.Message{ID: newID(), SessionID: session.ID, Role: atom.RoleRuntime, Content: []atom.Content{{Type: atom.Text, Text: text}}, CreatedAt: time.Now()}
	response, operationError := coordinator.requestSessionCommand(operationContext, sessionCommand{kind: submitFencedRuntime, control: control, message: message, permission: decision})
	return response.applied, operationError
}

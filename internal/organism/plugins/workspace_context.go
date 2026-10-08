package plugins

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/workspaceread"
)

func (services *Services) ReadTaskState(operationContext context.Context, workspaceID string, sessionID atom.SessionID) (atom.TaskState, error) {
	if operationError := services.validateContextSession(operationContext, workspaceID, sessionID); operationError != nil {
		return atom.TaskState{}, operationError
	}
	return services.Store.TaskStates().Get(operationContext, sessionID)
}

func (services *Services) SearchWorkspaceFiles(operationContext context.Context, query harness.WorkspaceFileQuery) (harness.WorkspaceFileResult, error) {
	if operationError := services.validateContextSession(operationContext, query.WorkspaceID, query.SourceSessionID); operationError != nil {
		return harness.WorkspaceFileResult{}, operationError
	}
	workspace, operationError := services.Workspace(operationContext, query.WorkspaceID)
	if operationError != nil {
		return harness.WorkspaceFileResult{}, operationError
	}
	return workspaceread.Search(operationContext, workspace.Workspace, query)
}

func (services *Services) VerifyWorkspaceFiles(operationContext context.Context, workspaceID string, sessionID atom.SessionID, references []harness.WorkspaceFileReference) (bool, error) {
	if operationError := services.validateContextSession(operationContext, workspaceID, sessionID); operationError != nil {
		return false, operationError
	}
	workspace, operationError := services.Workspace(operationContext, workspaceID)
	if operationError != nil {
		return false, operationError
	}
	return workspaceread.Verify(operationContext, workspace.Workspace, references)
}

func (services *Services) validateContextSession(operationContext context.Context, workspaceID string, sessionID atom.SessionID) error {
	if workspaceID == "" || sessionID == "" {
		return harness.ErrConversationUnavailable
	}
	session, operationError := services.Get(operationContext, sessionID)
	if operationError != nil {
		return operationError
	}
	if session.InstanceID != workspaceID {
		return harness.ErrConversationUnavailable
	}
	return nil
}

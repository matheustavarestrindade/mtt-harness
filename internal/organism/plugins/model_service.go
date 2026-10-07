package plugins

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/contextbuilder"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
)

// Services adapts existing runtime owners to public plugin contracts. It does
// not create sessions, register providers, run tools, or bypass model allowlists.
type Services struct {
	Gateway   *gateway.Gateway
	Instances *instances.Manager
	Store     store.Store
}

func (services *Services) Workspace(operationContext context.Context, identifier string) (atom.InstanceSpec, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.InstanceSpec{}, operationError
	}
	instance, found := services.Instances.Get(identifier)
	if !found {
		return atom.InstanceSpec{}, fmt.Errorf("workspace %q is not running", identifier)
	}
	return instance.Spec(), nil
}

func (services *Services) Model(operationContext context.Context, workspaceID, identifier, effort string) (atom.ModelInfo, error) {
	model, _, operationError := services.resolveAgentModel(operationContext, workspaceID, identifier, effort)
	return model, operationError
}

func (services *Services) resolveAgentModel(operationContext context.Context, workspaceID, identifier, effort string) (atom.ModelInfo, harness.Provider, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return atom.ModelInfo{}, nil, operationError
	}
	var allowed []string
	if workspaceID != "" {
		workspace, operationError := services.Workspace(operationContext, workspaceID)
		if operationError != nil {
			return atom.ModelInfo{}, nil, operationError
		}
		allowed = workspace.Models
	}
	if !strings.Contains(identifier, "/") {
		return atom.ModelInfo{}, nil, fmt.Errorf("worker model %q is not provider-qualified", identifier)
	}
	model, provider, operationError := services.Gateway.ResolveAllowed(identifier, allowed)
	if operationError != nil {
		return model, provider, operationError
	}
	if operationError := model.ValidateReasoningEffort(effort); operationError != nil {
		return model, provider, operationError
	}
	return model, provider, nil
}

func (services *Services) Run(operationContext context.Context, input harness.WorkspaceAgentRequest) (response harness.WorkspaceAgentResponse, operationError error) {
	if input.WorkspaceID == "" || input.Agent == "" || input.RunID == "" || input.RequestID == "" {
		return response, fmt.Errorf("workspace agent request requires workspace, agent, run and request IDs")
	}
	model, provider, operationError := services.resolveAgentModel(operationContext, input.WorkspaceID, input.Model, input.ReasoningEffort)
	if operationError != nil {
		return response, operationError
	}
	if model.ContextMax <= 0 {
		return response, fmt.Errorf("worker model %q has no known context limit", input.Model)
	}
	if input.MaxOutputTokens <= 0 {
		return response, fmt.Errorf("worker output reservation must be positive")
	}
	request := atom.Request{Model: model.ID, Messages: input.Messages, ReasoningEffort: input.ReasoningEffort, Params: map[string]any{"max_output_tokens": input.MaxOutputTokens}}
	budget, operationError := contextbuilder.MeasureRequest(operationContext, request, model, provider)
	if operationError != nil {
		return response, operationError
	}
	if budget.InputTokens > budget.InputLimit {
		return response, fmt.Errorf("worker input uses %d tokens; input budget is %d", budget.InputTokens, budget.InputLimit)
	}
	operationContext, cancel := context.WithCancel(harness.WithSession(operationContext, atom.Session{InstanceID: input.WorkspaceID}))
	defer cancel()
	started := time.Now()
	usageProvided := false
	response.Model = provider.Name() + "/" + model.ID
	defer func() {
		if usageProvided {
			response.Usage.Cost = atom.CalculateUsageCost(model, &response.Usage)
		}
		status := "completed"
		if operationError != nil {
			status = "error"
		}
		recordContext, cancelRecord := context.WithTimeout(context.WithoutCancel(operationContext), 10*time.Second)
		defer cancelRecord()
		operationError = errors.Join(operationError, services.Store.Usage().Save(recordContext, atom.UsageRecord{InstanceID: input.WorkspaceID, RequestID: input.RequestID, Agent: input.Agent, RunID: input.RunID, SourceSessionID: input.SourceSessionID, ModelID: response.Model, Usage: response.Usage, CreatedAt: started, Duration: time.Since(started), Status: status}))
	}()
	stream, operationError := provider.Stream(operationContext, request)
	if operationError != nil {
		return response, operationError
	}
	var text strings.Builder
	for {
		part, streamError := stream.Recv(operationContext)
		if part.Usage != nil {
			response.Usage = *part.Usage
			usageProvided = true
		}
		if errors.Is(streamError, io.EOF) {
			response.Text = text.String()
			return response, nil
		}
		if streamError != nil {
			return response, streamError
		}
		if part.ToolCall != nil {
			return response, fmt.Errorf("workspace agent returned a tool call without tool access")
		}
		if text.Len()+len(part.Text) > 256*1024 {
			return response, fmt.Errorf("workspace agent response exceeds 256 KiB")
		}
		text.WriteString(part.Text)
	}
}

// Get and Messages implement the read-only conversation service. Opaque
// provider state never reaches a plugin archive or background model request.
func (services *Services) Get(operationContext context.Context, identifier atom.SessionID) (atom.Session, error) {
	session, operationError := services.Store.Sessions().Get(operationContext, identifier)
	if errors.Is(operationError, store.ErrSessionDeleted) || errors.Is(operationError, store.ErrSessionNotFound) {
		return session, fmt.Errorf("%w: %v", harness.ErrConversationUnavailable, operationError)
	}
	return session, operationError
}

func (services *Services) Messages(operationContext context.Context, identifier atom.SessionID) ([]atom.Message, error) {
	messages, operationError := services.Store.Sessions().Messages(operationContext, identifier)
	if operationError != nil {
		return nil, operationError
	}
	result := append([]atom.Message(nil), messages...)
	for index := range result {
		result[index].ProviderState = nil
	}
	return result, nil
}

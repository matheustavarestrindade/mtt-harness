package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (pluginHost *Host) BeginRequest(operationContext context.Context, session atom.Session) (context.Context, func(), error) {
	var releases []func()
	var once sync.Once
	release := func() {
		once.Do(func() {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
		})
	}
	for _, registered := range pluginHost.All() {
		plugin, supported := registered.(harness.RequestLifecyclePlugin)
		if !supported {
			continue
		}
		nextContext, releasePlugin, operationError := plugin.BeginRequest(operationContext, session)
		if operationError != nil {
			release()
			return operationContext, func() {}, operationError
		}
		if nextContext == nil || releasePlugin == nil {
			release()
			return operationContext, func() {}, fmt.Errorf("plugin %q returned an invalid request lease", registered.Name())
		}
		operationContext = nextContext
		releases = append(releases, releasePlugin)
	}
	return operationContext, release, nil
}

func (pluginHost *Host) PrepareContext(operationContext context.Context, input harness.ContextRequest) (harness.ContextSelection, error) {
	selection := harness.ContextSelection{Request: input.Request}
	managedBy := ""
	for _, registered := range pluginHost.All() {
		plugin, supported := registered.(harness.ContextPolicyPlugin)
		if !supported {
			continue
		}
		input.Request = selection.Request
		var operationError error
		input.Budget, operationError = input.Measure(operationContext, input.Request)
		if operationError != nil {
			return selection, operationError
		}
		invariants, operationError := requestInvariants(input.Request)
		if operationError != nil {
			return selection, operationError
		}
		next, operationError := plugin.PrepareContext(operationContext, input)
		if operationError != nil {
			return selection, operationError
		}
		after, operationError := requestInvariants(next.Request)
		if operationError != nil {
			return selection, operationError
		}
		if !bytes.Equal(invariants, after) {
			return selection, fmt.Errorf("plugin %q changed model request fields outside context selection", registered.Name())
		}
		if next.Managed && managedBy != "" {
			return selection, fmt.Errorf("context selection has multiple owners: %q and %q", managedBy, registered.Name())
		}
		if next.Managed {
			managedBy = registered.Name()
		}
		selection.Request = next.Request
		selection.Managed = selection.Managed || next.Managed
	}
	return selection, nil
}

func requestInvariants(request atom.Request) ([]byte, error) {
	// Capture bytes before the callback: comparing shared maps or slices after
	// a callback would miss an in-place mutation of the original request.
	request.Messages = nil
	return json.Marshal(request)
}

func (pluginHost *Host) EndTurn(operationContext context.Context, session atom.Session, status string) error {
	var operationError error
	for _, registered := range pluginHost.All() {
		if plugin, supported := registered.(harness.TurnLifecyclePlugin); supported {
			operationError = errors.Join(operationError, plugin.EndTurn(operationContext, session, status))
		}
	}
	return operationError
}

func (pluginHost *Host) BeginHistoryChange(operationContext context.Context, change harness.HistoryChange) (func(context.Context, bool) error, error) {
	var completions []func(context.Context, bool) error
	complete := func(completionContext context.Context, committed bool) error {
		var operationError error
		for index := len(completions) - 1; index >= 0; index-- {
			operationError = errors.Join(operationError, completions[index](completionContext, committed))
		}
		return operationError
	}
	for _, registered := range pluginHost.All() {
		plugin, supported := registered.(harness.HistoryLifecyclePlugin)
		if !supported {
			continue
		}
		completion, operationError := plugin.BeginHistoryChange(operationContext, change)
		if operationError != nil {
			return nil, errors.Join(operationError, complete(context.WithoutCancel(operationContext), false))
		}
		if completion == nil {
			return nil, errors.Join(fmt.Errorf("plugin %q returned no history completion", registered.Name()), complete(context.WithoutCancel(operationContext), false))
		}
		completions = append(completions, completion)
	}
	return complete, nil
}

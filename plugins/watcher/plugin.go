package watcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Options struct {
	DatabaseURL string
	Services    harness.PluginServices
}
type activeGroup struct {
	workspaceID string
	sessionID   atom.SessionID
	cancel      context.CancelFunc
	done        chan struct{}
	runs        map[string]bool
}
type Plugin struct {
	services           harness.PluginServices
	database           repository
	unavailable        error
	operationContext   context.Context
	cancel             context.CancelFunc
	mutex              sync.Mutex
	configurationMutex sync.Mutex
	inbox              map[string]observation
	order              []string
	active             map[string]*activeGroup
	paused             map[atom.SessionID]bool
	errors             map[string]string
	changed            chan struct{}
	workers            sync.WaitGroup
	unsubscribe        harness.Unsubscribe
	closed             bool
}

func New(operationContext context.Context, options Options) (*Plugin, error) {
	if options.Services.Settings == nil || options.Services.Sessions == nil || options.Services.Models == nil || options.Services.Usage == nil || options.Services.Events == nil {
		return nil, fmt.Errorf("watcher services are incomplete")
	}
	pluginContext, cancel := context.WithCancel(operationContext)
	plugin := &Plugin{services: options.Services, operationContext: pluginContext, cancel: cancel, inbox: map[string]observation{}, active: map[string]*activeGroup{}, paused: map[atom.SessionID]bool{}, errors: map[string]string{}, changed: make(chan struct{}, 1)}
	database, operationError := openRepository(operationContext, options.DatabaseURL)
	if operationError != nil {
		plugin.unavailable = operationError
	} else {
		plugin.database = database
	}
	return plugin, nil
}
func (*Plugin) Name() string    { return pluginName }
func (*Plugin) Version() string { return "0.1.0" }
func (plugin *Plugin) Setup(runtime *harness.Harness) error {
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if plugin.closed || plugin.unsubscribe != nil {
		return fmt.Errorf("watcher is already attached or closed")
	}
	plugin.unsubscribe = runtime.On(atom.EventPermissionRequest, plugin.observePermission)
	if plugin.database != nil {
		for index := 0; index < 4; index++ {
			plugin.workers.Add(1)
			go plugin.runWorker()
		}
	}
	return nil
}
func (plugin *Plugin) Close(context.Context) error {
	plugin.mutex.Lock()
	if !plugin.closed {
		plugin.closed = true
		plugin.cancel()
		for _, group := range plugin.active {
			group.cancel()
		}
	}
	plugin.mutex.Unlock()
	plugin.workers.Wait()
	if plugin.unsubscribe != nil {
		plugin.unsubscribe()
	}
	if plugin.database != nil {
		plugin.database.Close()
	}
	return nil
}
func observationKey(value observation) string {
	suffix := ""
	if value.Permission != nil {
		suffix = value.Permission.ID
	}
	return string(value.Session.ID) + ":" + value.Trigger + ":" + suffix
}
func (plugin *Plugin) enqueue(value observation) {
	if value.Session.Execution != nil || value.Session.Parent != "" || value.Session.ID == "" {
		return
	}
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	if plugin.closed || plugin.database == nil || plugin.paused[value.Session.ID] {
		return
	}
	key := observationKey(value)
	if _, found := plugin.inbox[key]; !found {
		if len(plugin.inbox) >= 1024 {
			plugin.errors[value.Session.InstanceID] = "watcher pending review limit reached"
			return
		}
		plugin.order = append(plugin.order, key)
	}
	plugin.inbox[key] = value
	select {
	case plugin.changed <- struct{}{}:
	default:
	}
}
func (plugin *Plugin) observePermission(operationContext context.Context, event atom.Event) {
	var metadata struct {
		ID         string
		SessionID  atom.SessionID
		InstanceID string
	}
	if json.Unmarshal(event.Payload, &metadata) != nil || metadata.SessionID != event.SessionID {
		return
	}
	session, found := harness.SessionFrom(operationContext)
	if !found || session.ID != event.SessionID {
		return
	}
	if session.Execution != nil || session.Parent != "" {
		return
	}
	plugin.enqueue(observation{Session: session, Trigger: "permission_request", Permission: &atom.PermissionRequest{ID: metadata.ID, SessionID: metadata.SessionID, InstanceID: metadata.InstanceID}})
}

type requestConfiguration struct {
	configuration configuration
	state         sessionState
}
type requestKey struct{ plugin *Plugin }

func (plugin *Plugin) BeginRequest(operationContext context.Context, session atom.Session) (context.Context, func(), error) {
	if session.Execution != nil || session.Parent != "" || plugin.database == nil {
		return operationContext, func() {}, nil
	}
	configuration, _, operationError := plugin.loadConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		plugin.recordError(session.InstanceID, operationError)
		return operationContext, func() {}, nil
	}
	state, operationError := plugin.database.Session(operationContext, session.InstanceID, string(session.ID))
	if operationError != nil {
		return operationContext, func() {}, operationError
	}
	return context.WithValue(operationContext, requestKey{plugin}, requestConfiguration{configuration: configuration, state: state}), func() {}, nil
}
func (plugin *Plugin) PrepareReminder(operationContext context.Context, input harness.ContextRequest) (harness.ReminderContribution, error) {
	if input.Session.Execution != nil || input.Session.Parent != "" {
		return harness.ReminderContribution{}, nil
	}
	frozen, found := operationContext.Value(requestKey{plugin}).(requestConfiguration)
	if !found {
		return harness.ReminderContribution{}, nil
	}
	observeGrowth := frozen.configuration.Enabled && slices.ContainsFunc(frozen.configuration.Watchers, func(value definition) bool { return value.Enabled && slices.Contains(value.Triggers, "context_growth") })
	if !observeGrowth && len(frozen.state.Notices) == 0 {
		return harness.ReminderContribution{}, nil
	}
	var visible []string
	for _, message := range input.Request.Messages {
		if message.ID != "" && !message.Ephemeral {
			visible = append(visible, message.ID)
		}
	}
	value := observation{Session: input.Session, Trigger: "context_growth", Model: input.ModelID}
	if observeGrowth {
		measurement, growthVisible, operationError := measureContextGrowth(operationContext, input)
		if operationError != nil {
			return harness.ReminderContribution{}, operationError
		}
		value.Tokens = measurement.InputTokens
		value.Estimated = measurement.Estimated
		value.Visible = growthVisible
	}
	retired := map[string]bool{}
	for _, notice := range frozen.state.Notices {
		if !slices.Contains(visible, notice.AnchorID) {
			retired[notice.ID] = true
		}
	}
	return harness.ReminderContribution{Commit: func(commitContext context.Context, _ harness.ReminderDelivery) error {
		if len(retired) > 0 {
			if operationError := plugin.database.UpdateSession(commitContext, input.Session.InstanceID, string(input.Session.ID), func(state *sessionState) error {
				state.Notices = slices.DeleteFunc(state.Notices, func(notice reviewNotice) bool { return retired[notice.ID] })
				return nil
			}); operationError != nil {
				return operationError
			}
		}
		if observeGrowth {
			plugin.enqueue(value)
		}
		return nil
	}}, nil
}
func (plugin *Plugin) EndTurn(operationContext context.Context, session atom.Session, status string) error {
	if session.Execution != nil || session.Parent != "" || plugin.database == nil {
		return nil
	}
	if status != "completed" {
		plugin.cancelSession(session.ID)
		return nil
	}
	configuration, _, operationError := plugin.loadConfiguration(operationContext, session.InstanceID)
	if operationError != nil {
		plugin.recordError(session.InstanceID, operationError)
		return nil
	}
	if !configuration.Enabled {
		return nil
	}
	if !slices.ContainsFunc(configuration.Watchers, func(value definition) bool { return value.Enabled && slices.Contains(value.Triggers, "end_turn") }) {
		return nil
	}
	source, operationError := plugin.services.Sessions.Source(operationContext, session.ID, 1024, false)
	if operationError != nil {
		plugin.recordError(session.InstanceID, operationError)
		return nil
	}
	plugin.enqueue(observation{Session: session, Trigger: "end_turn", Fence: &source.Fence, Head: source.HeadSequence})
	return nil
}
func (plugin *Plugin) recordError(workspaceID string, operationError error) {
	if operationError == nil || errors.Is(operationError, harness.ErrReviewStale) || errors.Is(operationError, context.Canceled) {
		return
	}
	plugin.mutex.Lock()
	plugin.errors[workspaceID] = operationError.Error()
	plugin.mutex.Unlock()
}
func (plugin *Plugin) cancelSession(sessionID atom.SessionID) {
	plugin.mutex.Lock()
	defer plugin.mutex.Unlock()
	for key, value := range plugin.inbox {
		if value.Session.ID == sessionID {
			delete(plugin.inbox, key)
		}
	}
	plugin.compactInboxLocked()
	for _, group := range plugin.active {
		if group.sessionID == sessionID {
			group.cancel()
		}
	}
}
func (plugin *Plugin) compactInboxLocked() {
	retained := plugin.order[:0]
	for _, key := range plugin.order {
		if _, found := plugin.inbox[key]; found {
			retained = append(retained, key)
		}
	}
	plugin.order = retained
}

var _ harness.Plugin = (*Plugin)(nil)
var _ harness.RequestLifecyclePlugin = (*Plugin)(nil)
var _ harness.RequestReminderPlugin = (*Plugin)(nil)
var _ harness.TurnLifecyclePlugin = (*Plugin)(nil)
var _ harness.HistoryLifecyclePlugin = (*Plugin)(nil)
var _ harness.ConfigurablePlugin = (*Plugin)(nil)
var _ harness.ObservablePlugin = (*Plugin)(nil)
var _ harness.SessionActivityPlugin = (*Plugin)(nil)
var _ harness.PluginActions = (*Plugin)(nil)

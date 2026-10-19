package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/gateway"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/registry"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type failingViewTasks struct{ store.TaskStateStore }

func (failingViewTasks) Get(context.Context, atom.SessionID) (atom.TaskState, error) {
	return atom.TaskState{}, errors.New("progress store unavailable")
}

type partialViewStore struct{ store.Store }

func (database partialViewStore) TaskStates() store.TaskStateStore {
	return failingViewTasks{database.Store.TaskStates()}
}

func TestSessionViewRetainsConversationWhenTaskStateFails(test *testing.T) {
	database := memory.New()
	session := atom.Session{ID: "partial-view", InstanceID: "partial-workspace"}
	testutil.RequireNoError(test, database.Instances().Save(context.Background(), atom.InstanceSpec{ID: session.InstanceID}))
	testutil.RequireNoError(test, database.Sessions().Save(context.Background(), session))
	testutil.RequireNoError(test, database.Sessions().Append(context.Background(), atom.Message{ID: "preserved", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Preserved history"}}}))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: partialViewStore{database}}).Handler())
	defer server.Close()
	response := request(test, server.URL+"/sessions/partial-view/state", "secret", "GET", nil)
	if response.StatusCode != 200 {
		test.Fatal(response.Status)
	}
	var state struct {
		Session        atom.Session
		TaskState      *atom.TaskState
		TaskStateError string
		History        atom.HistoryPage
	}
	decode(test, response, &state)
	if state.Session.ID != session.ID || state.TaskState != nil || state.TaskStateError != "progress store unavailable" || state.History.HeadSeq == 0 {
		test.Fatalf("optional failure discarded state: %+v", state)
	}
}

type viewProvider struct{ resume chan struct{} }

func (*viewProvider) Name() string { return "view" }
func (*viewProvider) Models() []atom.ModelInfo {
	return []atom.ModelInfo{{ID: "view-model", Input: []atom.MediaType{atom.Text}, Tools: true, ContextMax: 100000}}
}
func (provider *viewProvider) Stream(context.Context, atom.Request) (harness.Stream, error) {
	return &viewResponseStream{resume: provider.resume}, nil
}

type viewResponseStream struct {
	resume chan struct{}
	step   int
}

func (stream *viewResponseStream) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	stream.step++
	if stream.step == 1 {
		return atom.ResponsePart{Text: "first"}, nil
	}
	if stream.step == 2 {
		select {
		case <-stream.resume:
			return atom.ResponsePart{Text: " second"}, nil
		case <-operationContext.Done():
			return atom.ResponsePart{}, operationContext.Err()
		}
	}
	return atom.ResponsePart{}, io.EOF
}

func TestSessionViewPushesQueueStateAndUsesBoundedLiveSnapshots(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database := memory.New()
	runtime := harness.New()
	bus := eventbus.New(runtime)
	models := gateway.New(runtime)
	tools := registry.New(runtime, nil)
	provider := &viewProvider{resume: make(chan struct{})}
	resume := sync.OnceFunc(func() { close(provider.resume) })
	defer resume()
	testutil.RequireNoError(test, models.Add(provider))
	manager := instances.New(func(string) instances.SessionManager { return nil }, nil)
	instance, operationError := manager.Start(operationContext, atom.InstanceSpec{ID: "view-workspace", Workspace: test.TempDir(), DefaultModel: "view/view-model"})
	testutil.RequireNoError(test, operationError)
	testutil.RequireNoError(test, database.Instances().Save(operationContext, instance.Spec()))
	session := atom.Session{ID: "view-session", InstanceID: instance.Spec().ID, Model: "view/view-model", CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	runner := loop.New(runtime, loop.Config{Store: database, Gateway: models, Registry: tools, Instances: manager, Bus: bus})
	queue := loop.NewQueue(runner)
	defer queue.Close(context.Background())
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Instances: manager, Gateway: models, Loop: runner, Queue: queue, Bus: bus}).Handler())
	defer server.Close()
	for index := 0; index < 500; index++ {
		testutil.RequireNoError(test, bus.Send(operationContext, atom.Event{SessionID: session.ID, InstanceID: session.InstanceID, Name: atom.EventModelChunk, Payload: json.RawMessage(`{"message_id":"old","text":"old history chunk"}`), Time: time.Now()}))
	}
	firstChunk := make(chan struct{})
	once := sync.OnceFunc(func() { close(firstChunk) })
	unsubscribe := bus.On(atom.EventModelChunk, func(_ context.Context, event atom.Event) {
		if strings.Contains(string(event.Payload), `"text":"first"`) {
			once()
		}
	})
	defer unsubscribe()
	response := request(test, server.URL+"/sessions/view-session/messages", "secret", "POST", map[string]any{"content": "start"})
	if response.StatusCode != 202 {
		test.Fatal(response.Status)
	}
	response.Body.Close()
	<-firstChunk
	connection, _, operationError := websocket.Dial(operationContext, "ws"+strings.TrimPrefix(server.URL, "http")+"/sessions/view-session/events?token=secret&view=true", nil)
	testutil.RequireNoError(test, operationError)
	defer connection.CloseNow()
	read := func() map[string]json.RawMessage {
		test.Helper()
		_, data, operationError := connection.Read(operationContext)
		testutil.RequireNoError(test, operationError)
		var frame map[string]json.RawMessage
		testutil.RequireNoError(test, json.Unmarshal(data, &frame))
		return frame
	}
	initial := read()
	if string(initial["type"]) != `"state"` {
		test.Fatalf("missing first snapshot: %s", initial["type"])
	}
	var state struct {
		Session atom.Session
		Status  struct {
			Running bool
			Queued  int
		}
		Live    *atom.LiveResponse
		History atom.HistoryPage
	}
	testutil.RequireNoError(test, json.Unmarshal(initial["state"], &state))
	if !state.Status.Running || state.Live == nil || state.Live.Revision != 1 || state.Live.Message.Content[0].Text != "first" {
		test.Fatalf("incomplete live snapshot: %+v", state)
	}
	var recent []atom.Event
	testutil.RequireNoError(test, json.Unmarshal(initial["recent"], &recent))
	if len(recent) > 80 {
		test.Fatal("full event history was replayed")
	}
	for _, event := range recent {
		if event.Name == atom.EventModelChunk {
			test.Fatal("old output chunks entered the activity tail")
		}
	}
	queued := request(test, server.URL+"/sessions/view-session/messages", "secret", "POST", map[string]any{"content": "waiting"})
	var receipt struct {
		Message atom.Message `json:"message"`
	}
	decode(test, queued, &receipt)
	awaitState := func(predicate func() bool) {
		test.Helper()
		for {
			frame := read()
			if string(frame["type"]) != `"state"` {
				continue
			}
			testutil.RequireNoError(test, json.Unmarshal(frame["state"], &state))
			if predicate() {
				return
			}
		}
	}
	awaitState(func() bool { return state.Status.Queued == 1 && state.Status.Running })
	removed := request(test, server.URL+"/sessions/view-session/queue/"+receipt.Message.ID, "secret", "DELETE", nil)
	removed.Body.Close()
	awaitState(func() bool { return state.Status.Queued == 0 && state.Status.Running })
	resume()
	awaitState(func() bool { return !state.Status.Running && state.History.LatestResponse != nil })
	if state.History.LatestResponse.MessageID == "" || state.Live != nil {
		test.Fatal("saved response did not retire live state")
	}
}

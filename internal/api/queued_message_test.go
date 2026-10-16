package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/api"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/instances"
	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestQueuedMessagesRestoreThroughStateAndAuthenticatedRead(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	database := memory.New()
	instance := atom.InstanceSpec{ID: "queue-preview-workspace", Workspace: test.TempDir(), Stopped: true}
	session := atom.Session{ID: "queue-preview-session", InstanceID: instance.ID, CreatedAt: time.Now()}
	testutil.RequireNoError(test, database.Instances().Save(operationContext, instance))
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	text := atom.Message{ID: "pending-text", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: "Restored from the durable queue 🌍"}}, CreatedAt: time.Now()}
	large := atom.Message{ID: "pending-media", SessionID: session.ID, Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("long text ", 3000)}, {Type: atom.Image, MIME: "image/png", Filename: "queued.png", Data: []byte{0, 1, 2, 255}}}, CreatedAt: time.Now()}
	for _, message := range []atom.Message{text, large} {
		testutil.RequireNoError(test, database.Queue().Enqueue(operationContext, message, 128))
	}
	runtime := harness.New()
	bus := eventbus.New(runtime)
	manager := instances.New(func(string) instances.SessionManager { return nil }, nil)
	queue := loop.NewQueue(loop.New(runtime, loop.Config{Store: database, Instances: manager, Bus: bus}))
	defer queue.Close(context.Background())
	testutil.RequireNoError(test, queue.Restore(operationContext))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Queue: queue, Bus: bus}).Handler())
	defer server.Close()
	var state struct {
		QueuedMessages []atom.QueuedMessagePreview
		Status         struct {
			Messages []string `json:"messages"`
		}
	}
	response := request(test, server.URL+"/sessions/queue-preview-session/state", "secret", http.MethodGet, nil)
	decode(test, response, &state)
	if len(state.QueuedMessages) != 2 || state.QueuedMessages[0].Content[0].Text != text.Content[0].Text || state.QueuedMessages[0].Role != atom.RoleUser || !state.QueuedMessages[1].Truncated {
		test.Fatalf("queue content was not restored: %+v", state)
	}
	response = request(test, server.URL+"/sessions/queue-preview-session/state?details=false", "secret", http.MethodGet, nil)
	var compact map[string]json.RawMessage
	decode(test, response, &compact)
	if _, found := compact["QueuedMessages"]; found {
		test.Fatal("metadata-only view loaded queued bodies")
	}
	for attempt := 0; attempt < 2; attempt++ {
		socket, _, operationError := websocket.Dial(operationContext, "ws"+strings.TrimPrefix(server.URL, "http")+"/sessions/queue-preview-session/events?view=true&token=secret", nil)
		testutil.RequireNoError(test, operationError)
		_, data, operationError := socket.Read(operationContext)
		testutil.RequireNoError(test, operationError)
		var frame struct {
			Type  string                                               `json:"type"`
			State struct{ QueuedMessages []atom.QueuedMessagePreview } `json:"state"`
		}
		testutil.RequireNoError(test, json.Unmarshal(data, &frame))
		if frame.Type != "state" || len(frame.State.QueuedMessages) != 2 {
			test.Fatal("reconnected socket lost queue content")
		}
		socket.CloseNow()
	}
	url := server.URL + "/sessions/queue-preview-session/queue/pending-media"
	response = request(test, url, "wrong", http.MethodGet, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		test.Fatal("pending-message read did not require authentication")
	}
	response = request(test, url, "secret", http.MethodGet, nil)
	var restored atom.Message
	decode(test, response, &restored)
	if restored.Content[0].Text != large.Content[0].Text || string(restored.Content[1].Data) != string(large.Content[1].Data) {
		test.Fatal("full pending content changed")
	}
	entries, operationError := database.Queue().All(operationContext)
	testutil.RequireNoError(test, operationError)
	if len(entries) != 2 || entries[0].Running || entries[1].Running {
		test.Fatal("reading pending input started a turn")
	}
	messages, operationError := database.Sessions().Messages(operationContext, session.ID)
	testutil.RequireNoError(test, operationError)
	if len(messages) != 0 {
		test.Fatal("queue read appended conversation history")
	}
	response = request(test, url, "secret", http.MethodDelete, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		test.Fatal(response.Status)
	}
	response = request(test, url, "secret", http.MethodGet, nil)
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		test.Fatal("removed input remained readable as pending")
	}
}

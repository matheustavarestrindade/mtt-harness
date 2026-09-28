package api_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestWebSocketReplayAndLiveEventsShareSequence(test *testing.T) {
	operationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	database := memory.New()
	runtime := harness.New()
	bus := eventbus.New(runtime)
	session := atom.Session{ID: "session", InstanceID: "instance"}
	testutil.RequireNoError(test, database.Sessions().Save(operationContext, session))
	server := httptest.NewServer(api.New(api.Config{Token: "secret", Store: database, Bus: bus}).Handler())
	defer server.Close()
	send := func() {
		testutil.RequireNoError(test, bus.Send(operationContext, atom.Event{Name: "test", InstanceID: session.InstanceID, SessionID: session.ID, Time: time.Now()}))
	}
	connect := func(since uint64) *websocket.Conn {
		connection, _, operationError := websocket.Dial(operationContext, strings.Replace(server.URL, "http://", "ws://", 1)+fmt.Sprintf("/sessions/session/events?token=secret&since=%d", since), nil)
		testutil.RequireNoError(test, operationError)
		return connection
	}
	read := func(connection *websocket.Conn) atom.Event {
		_, data, operationError := connection.Read(operationContext)
		testutil.RequireNoError(test, operationError)
		var event atom.Event
		testutil.RequireNoError(test, json.Unmarshal(data, &event))
		return event
	}
	send()
	first := connect(0)
	eventOne := read(first)
	send()
	eventTwo := read(first)
	first.Close(websocket.StatusNormalClosure, "")
	send()
	second := connect(eventOne.Seq)
	defer second.CloseNow()
	replayed := read(second)
	eventThree := read(second)
	if replayed.Seq != eventTwo.Seq || eventThree.Seq <= eventTwo.Seq {
		test.Fatalf("reconnect lost event ordering: %d %d %d", eventTwo.Seq, replayed.Seq, eventThree.Seq)
	}
	send()
	eventFour := read(second)
	if eventFour.Seq <= eventThree.Seq {
		test.Fatal("live event reused an old sequence")
	}
}

package processes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/eventbus"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/process"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/store/memory"
)

func TestManagerSendsNotification(t *testing.T) {
	database := memory.New()
	bus := eventbus.New()
	h := harness.New()
	supervisor := process.New(4)
	manager := New(supervisor, database, h, bus)
	session := atom.Session{ID: "session-1", InstanceID: "instance-1", CreatedAt: time.Now()}
	if err := database.Sessions().Save(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	ctx := harness.WithSession(context.Background(), session)
	proc, err := manager.Start(ctx, atom.ProcessSpec{
		Command: "sh",
		Args:    []string{"-c", "echo ready"},
		Notify:  atom.NotifyPolicy{Mode: atom.NotifyExit},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := proc.Wait(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		messages, _ := database.Sessions().Messages(context.Background(), session.ID)
		if len(messages) > 0 && strings.Contains(messages[0].Content[0].Text, "[process]") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the notification message did not arrive")
}

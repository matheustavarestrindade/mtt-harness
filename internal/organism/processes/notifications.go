package processes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (processManager *Manager) tail(runningProcess harness.Process) string {
	stdout, stderr, _ := processManager.Output(runningProcess.ID())
	if len(stderr) > 0 {
		return string(stderr)
	}
	return string(stdout)
}

func (processManager *Manager) send(operationContext context.Context, session atom.Session, text string) error {
	if session.ID == "" {
		return nil
	}
	processManager.mutex.Lock()
	notifier, closing := processManager.notifier, processManager.closing
	processManager.mutex.Unlock()
	if closing {
		return nil
	}
	content := []atom.Content{{Type: atom.Text, Text: "[process] " + text}}
	if notifier != nil {
		if operationError := notifier(operationContext, session, content); operationError != nil {
			return operationError
		}
	}
	if notifier == nil {
		var identifier [16]byte
		if _, operationError := rand.Read(identifier[:]); operationError != nil {
			return operationError
		}
		message := atom.Message{ID: hex.EncodeToString(identifier[:]), SessionID: session.ID, Role: atom.RoleUser, Content: content, CreatedAt: time.Now()}
		if operationError := processManager.store.Sessions().Append(operationContext, message); operationError != nil {
			return operationError
		}
	}
	if processManager.bus != nil {
		return processManager.bus.Send(operationContext, atom.Event{InstanceID: session.InstanceID, SessionID: session.ID, Name: atom.EventProcessNotify, Time: time.Now()})
	}
	return nil
}

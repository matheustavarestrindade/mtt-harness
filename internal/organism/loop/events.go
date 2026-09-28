package loop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

func (agentLoop *Loop) emit(operationContext context.Context, session atom.Session, name atom.EventName, payload any) error {
	operationContext = harness.WithSession(operationContext, session)
	var raw json.RawMessage
	if payload != nil {
		var operationError error
		raw, operationError = json.Marshal(payload)
		if operationError != nil {
			return operationError
		}
	}
	event := atom.Event{
		InstanceID: session.InstanceID,
		SessionID:  session.ID,
		Name:       name,
		Payload:    raw,
		Time:       time.Now(),
	}
	if agentLoop.configuration.Bus != nil {
		return agentLoop.configuration.Bus.Send(operationContext, event)
	}
	return nil
}

func newID() string {
	var data [16]byte
	if _, operationError := rand.Read(data[:]); operationError != nil {
		return "unknown"
	}
	return hex.EncodeToString(data[:])
}

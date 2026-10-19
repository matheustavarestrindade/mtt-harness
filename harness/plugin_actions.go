package harness

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrPluginActionNotFound = errors.New("plugin action is not available")

// PluginActions exposes workspace-scoped, plugin-owned operations through the
// authenticated API. Implementations own input validation and side-effect rules.
type PluginActions interface {
	Action(context.Context, string, string, json.RawMessage) (any, error)
}

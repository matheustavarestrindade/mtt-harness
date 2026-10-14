package harness

import "context"

type toolOriginKey struct{}

// ToolOrigin identifies one call occurrence, including providers that reuse call
// IDs in later model responses. It is runtime metadata, not a model argument.
type ToolOrigin struct {
	MessageID string
	CallID    string
}

// WithToolOrigin carries the executing call's identity to process services.
func WithToolOrigin(operationContext context.Context, origin ToolOrigin) context.Context {
	return context.WithValue(operationContext, toolOriginKey{}, origin)
}

// ToolOriginFrom returns false outside runtime tool-call execution.
func ToolOriginFrom(operationContext context.Context) (ToolOrigin, bool) {
	origin, found := operationContext.Value(toolOriginKey{}).(ToolOrigin)
	return origin, found
}

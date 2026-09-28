package harness

// Plugin attaches its capabilities before the runtime begins processing turns.
type Plugin interface {
	Name() string
	Version() string
	Setup(harnessRuntime *Harness) error
}

package harness

type Plugin interface {
	Name() string
	Version() string
	Setup(h *Harness) error
}

package bridge

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Bridge struct {
	h *harness.Harness
}

func New(h *harness.Harness) *Bridge {
	return &Bridge{h: h}
}

func (b *Bridge) Start(ctx context.Context, command string, args ...string) error {
	return errors.New("bridge: not implemented")
}

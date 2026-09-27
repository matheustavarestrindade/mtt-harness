package harness

import (
	"context"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

type Provider interface {
	Name() string
	Models() []atom.ModelInfo
	Stream(ctx context.Context, request atom.Request) (Stream, error)
}

type Stream interface {
	Recv(ctx context.Context) (atom.ResponsePart, error)
}

type Refresher interface {
	Refresh(ctx context.Context) ([]atom.ModelInfo, error)
}

package provider

import (
	"context"
	"errors"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Standard struct {
	ProviderName  string
	APIURL        string
	ModelListURL  string
	PriceTableURL string
	Secret        string
	models        []atom.ModelInfo
}

func New(spec atom.ProviderSpec) *Standard {
	return &Standard{
		ProviderName:  spec.Name,
		APIURL:        spec.APIURL,
		ModelListURL:  spec.ModelListURL,
		PriceTableURL: spec.PriceTableURL,
		Secret:        spec.Secret,
	}
}

func (s *Standard) Name() string {
	return s.ProviderName
}

func (s *Standard) Models() []atom.ModelInfo {
	return append([]atom.ModelInfo(nil), s.models...)
}

func (s *Standard) Stream(ctx context.Context, request atom.Request) (harness.Stream, error) {
	return nil, errors.New("provider: stream is not implemented")
}

func (s *Standard) Refresh(ctx context.Context) ([]atom.ModelInfo, error) {
	return nil, errors.New("provider: refresh is not implemented")
}

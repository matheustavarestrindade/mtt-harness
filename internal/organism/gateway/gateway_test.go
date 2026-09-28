package gateway

import (
	"context"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type catalogProvider struct {
	name   string
	models []atom.ModelInfo
}

func (provider *catalogProvider) Name() string {
	return provider.name
}
func (provider *catalogProvider) Models() []atom.ModelInfo {
	return provider.models
}
func (provider *catalogProvider) Stream(context.Context, atom.Request) (harness.Stream, error) {
	panic("not called")
}

func TestQualifiedModelsDoNotCollide(test *testing.T) {
	runtime := harness.New()
	gateway := New(runtime)
	for _, provider := range []*catalogProvider{{"a", []atom.ModelInfo{{ID: "shared"}, {ID: "target/model"}}}, {"b", []atom.ModelInfo{{ID: "shared"}, {ID: "target/model"}}}, {"target", []atom.ModelInfo{{ID: "model"}}}} {
		testutil.RequireNoError(test, gateway.Add(provider))
	}
	if _, _, operationError := gateway.Resolve("shared"); operationError == nil {
		test.Fatal("ambiguous model silently selected a provider")
	}
	_, selected, operationError := gateway.Resolve("a/shared")
	testutil.RequireNoError(test, operationError)
	if selected.Name() != "a" {
		test.Fatal("wrong provider")
	}
	_, selected, operationError = gateway.Resolve("target/model")
	testutil.RequireNoError(test, operationError)
	if selected.Name() != "target" {
		test.Fatal("qualified ID lost to bare aliases")
	}
	if _, _, operationError := gateway.ResolveAllowed("b/shared", []string{"a/shared"}); operationError == nil {
		test.Fatal("model allowlist was bypassed")
	}
}

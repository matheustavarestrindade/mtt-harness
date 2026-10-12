package contextbuilder

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
	"github.com/matheustavarestrindade/mtt-harness/internal/molecule/provider"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func TestLongHistoryFitUsesBoundedMeasurementsAndKeepsProtectedMessages(test *testing.T) {
	request := atom.Request{Messages: []atom.Message{{ID: "startup", Role: atom.RoleSystem}}}
	for index := 0; index < 2000; index++ {
		request.Messages = append(request.Messages, atom.Message{ID: fmt.Sprint(index), Role: atom.RoleUser}, atom.Message{Role: atom.RoleAssistant, ToolCalls: []atom.ToolCall{{ID: fmt.Sprint(index)}}}, atom.Message{Role: atom.RoleTool, ToolCallID: fmt.Sprint(index)})
		if index == 5 {
			request.Messages = append(request.Messages, atom.Message{ID: "protected", Role: atom.RoleSystem, Ephemeral: true})
		}
	}
	calls := 0
	fitted, operationError := (&Builder{}).FitMeasured(context.Background(), request, func(operationContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		calls++
		return harness.RequestBudget{InputTokens: len(candidate.Messages), InputLimit: 14}, nil
	})
	testutil.RequireNoError(test, operationError)
	if calls > 16 {
		test.Fatalf("history fitting needed %d complete recounts", calls)
	}
	if len(fitted.Messages) != 14 || fitted.Messages[0].ID != "startup" || fitted.Messages[1].ID != "protected" || fitted.Messages[2].ID != "1996" {
		test.Fatalf("wrong fitting suffix: %+v", fitted.Messages)
	}
	if len(request.Messages) != 6002 || request.Messages[1].ID != "0" {
		test.Fatal("fitting changed history")
	}
}

func TestNonMonotoneCounterDoesNotRejectAFittingSuffix(test *testing.T) {
	request := atom.Request{Messages: []atom.Message{{ID: "old", Role: atom.RoleUser}, {ID: "fits", Role: atom.RoleUser}, {ID: "new", Role: atom.RoleUser}}}
	fitted, operationError := (&Builder{}).FitMeasured(context.Background(), request, func(operationContext context.Context, candidate atom.Request) (harness.RequestBudget, error) {
		tokens := 100
		if len(candidate.Messages) == 2 {
			tokens = 10
		}
		return harness.RequestBudget{InputTokens: tokens, InputLimit: 20}, nil
	})
	testutil.RequireNoError(test, operationError)
	if len(fitted.Messages) != 2 || fitted.Messages[0].ID != "fits" {
		test.Fatal("fitting suffix was discarded")
	}
}

func BenchmarkLongHistoryFit(benchmark *testing.B) {
	for _, turns := range []int{1000, 2000} {
		benchmark.Run(fmt.Sprint(turns), func(benchmark *testing.B) {
			request := atom.Request{}
			for index := 0; index < turns; index++ {
				request.Messages = append(request.Messages, atom.Message{Role: atom.RoleUser, Content: []atom.Content{{Type: atom.Text, Text: strings.Repeat("x", 128)}}})
			}
			model := atom.ModelInfo{ContextMax: 4096}
			modelProvider := provider.NewTest("benchmark")
			benchmark.ReportAllocs()
			benchmark.ResetTimer()
			for benchmark.Loop() {
				if _, operationError := (&Builder{}).Fit(context.Background(), request, model, modelProvider); operationError != nil {
					benchmark.Fatal(operationError)
				}
			}
		})
	}
}

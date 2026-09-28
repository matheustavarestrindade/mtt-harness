package harness

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func TestRegistrationRemovalUsesStableIdentity(test *testing.T) {
	runtime := New()
	var observed []int
	removeFirst := runtime.On("*", func(context.Context, atom.Event) {
		observed = append(observed, 0)
	})
	removeSecond := runtime.On("event", func(context.Context, atom.Event) {
		observed = append(observed, 1)
	})
	runtime.On("event", func(context.Context, atom.Event) {
		observed = append(observed, 2)
	})
	runtime.Emit(context.Background(), atom.Event{Name: "event"})
	if !reflect.DeepEqual(observed, []int{0, 1, 2}) {
		test.Fatalf("attachment order: %v", observed)
	}
	observed = nil
	removeFirst()
	removeSecond()
	removeSecond()
	runtime.Emit(context.Background(), atom.Event{Name: "event"})
	if !reflect.DeepEqual(observed, []int{2}) {
		test.Fatalf("remaining callbacks: %v", observed)
	}
	stage := atom.NewStage[string]("example")
	first := Pipe(runtime, stage, func(_ context.Context, value string) (string, error) {
		return value + "0", nil
	})
	second := Pipe(runtime, stage, func(_ context.Context, value string) (string, error) {
		return value + "1", nil
	})
	Pipe(runtime, stage, func(_ context.Context, value string) (string, error) {
		return value + "2", nil
	})
	first()
	second()
	second()
	result, operationError := Run(context.Background(), runtime, stage, "")
	if operationError != nil || result != "2" {
		test.Fatalf("remaining middleware: %q %v", result, operationError)
	}
}

func TestConcurrentRegistrationSnapshots(test *testing.T) {
	runtime := New()
	stage := atom.NewStage[int]("counter")
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for iteration := 0; iteration < 100; iteration++ {
				unsubscribe := Pipe(runtime, stage, func(_ context.Context, value int) (int, error) {
					return value + 1, nil
				})
				_, _ = Run(context.Background(), runtime, stage, 0)
				unsubscribe()
				unsubscribe()
			}
		}()
	}
	workers.Wait()
	result, operationError := Run(context.Background(), runtime, stage, 0)
	if operationError != nil || result != 0 {
		test.Fatalf("leaked middleware: %d %v", result, operationError)
	}
}

package spacedrepetition

import (
	"testing"
)

func TestGrowthPatternCrossingsAndRebasing(test *testing.T) {
	configuration := defaultConfiguration()
	state := scheduleState{}
	visible := []string{"first"}
	for index, level := range []string{"low", "low", "low", "medium", "low", "low", "low", "medium"} {
		tokens := (index + 1) * 32768
		due := observeGrowth(&state, configuration, "fixture/model", 262144, tokens, visible, "v1")
		if due.level != level || due.crossed != 1 {
			test.Fatalf("checkpoint %d = %+v", index, due)
		}
		state.Next, state.Cursor = due.next, due.cursor
		if repeated := observeGrowth(&state, configuration, "fixture/model", 262144, tokens, visible, "v1"); repeated.level != "" {
			test.Fatal("unchanged context retriggered")
		}
	}
	before := state.Cursor
	if due := observeGrowth(&state, configuration, "fixture/model", 262144, 20000, []string{"retained"}, "v1"); due.level != "" || state.Cursor != before || state.Next != 32768 {
		test.Fatalf("compaction did not preserve phase and rebase: %+v %+v", state, due)
	}
	if due := observeGrowth(&state, configuration, "fixture/model", 262144, 32768, []string{"retained"}, "v1"); due.level != "low" {
		test.Fatalf("growth after compaction did not trigger: %+v", due)
	}
	if due := observeGrowth(&state, configuration, "fixture/smaller", 131072, 20000, []string{"retained"}, "v1"); due.level != "" || state.Step != 16384 || state.Cursor != before {
		test.Fatalf("model switch replayed thresholds: %+v %+v", state, due)
	}
}

func TestLargeJumpCoalescesMediumAndUnknownCapacityDefers(test *testing.T) {
	configuration := defaultConfiguration()
	state := scheduleState{}
	due := observeGrowth(&state, configuration, "fixture/model", 1048576, 180000, []string{"source"}, "v1")
	if due.level != "medium" || due.crossed != 5 || due.cursor != 5 || due.next != 196608 {
		test.Fatalf("jump not coalesced: %+v", due)
	}
	unknown := scheduleState{}
	if due := observeGrowth(&unknown, configuration, "fixture/unknown", 0, 999999, []string{"source"}, "v1"); due.level != "" {
		test.Fatal("unknown model capacity invented a threshold")
	}
}

func TestNestedConfigurationInheritanceAndNullRemoval(test *testing.T) {
	value, operationError := mergeConfiguration([]byte(`{"interval":{"fraction":0.2},"pattern":["medium","low"]}`), []byte(`{"interval":{"max_tokens":65536}}`))
	if operationError != nil || value.Interval.Fraction != 0.2 || value.Interval.MaxTokens != 65536 || value.Pattern[0] != "medium" {
		test.Fatalf("bad inherited config: %+v %v", value, operationError)
	}
	patched, operationError := patchConfiguration(`{"interval":{"fraction":0.3,"max_tokens":65536}}`, []byte(`{"interval":{"fraction":null}}`))
	if operationError != nil {
		test.Fatal(operationError)
	}
	value, operationError = mergeConfiguration([]byte(`{"interval":{"fraction":0.2}}`), patched)
	if operationError != nil || value.Interval.Fraction != 0.2 || value.Interval.MaxTokens != 65536 {
		test.Fatalf("nested null removed the wrong setting: %+v %v", value, operationError)
	}
	if _, operationError := mergeConfiguration(nil, []byte(`{"pattern":["high"]}`)); operationError == nil {
		test.Fatal("accepted automatic paid high recovery")
	}
}

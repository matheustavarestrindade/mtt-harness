package loop_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/organism/loop"
	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

func jsonUnmarshal(data []byte, value any) error {
	return json.Unmarshal(data, value)
}

func newTestQueue(test *testing.T, agentLoop *loop.Loop) *loop.Queue {
	test.Helper()
	queue := loop.NewQueue(agentLoop)
	test.Cleanup(func() {
		operationContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		testutil.RequireNoError(test, queue.Close(operationContext))
	})
	return queue
}

package contextplugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

// The first attempt saves a chunk, then exhausts its worker deadline. Subsequent
// attempts succeed so the checkpoint must wait through the persisted retry.
type checkpointRetryEmbeddings struct {
	testEmbeddings
	calls       atomic.Int32
	mutex       sync.Mutex
	firstChunk  string
	firstWrites int
}

func (embeddings *checkpointRetryEmbeddings) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	call := embeddings.calls.Add(1)
	if call == 2 {
		<-operationContext.Done()
		return nil, operationContext.Err()
	}
	embeddings.mutex.Lock()
	if call == 1 {
		embeddings.firstChunk = texts[0]
	}
	if texts[0] == embeddings.firstChunk {
		embeddings.firstWrites++
	}
	embeddings.mutex.Unlock()
	return embeddings.testEmbeddings.Embed(operationContext, texts)
}

func (embeddings *checkpointRetryEmbeddings) EmbedQueries(operationContext context.Context, texts []string) ([][]float64, error) {
	// Candidate retrieval can legitimately embed the same text as a source.
	// Count document writes separately to detect repeated indexing on retry.
	return embeddings.testEmbeddings.Embed(operationContext, texts)
}

func TestForcedCheckpointWaitsAcrossResumableWorkerDeadlines(test *testing.T) {
	embeddings := &checkpointRetryEmbeddings{}
	fixture := newPluginFixtureWithEmbeddings(test, embeddings)
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"job_timeout_ms":1000,"worker_count":1,"retry_limit":0}`))
	testutil.RequireNoError(test, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	input := weightedContextInput(fixture.session, messages, 3000)
	selection, operationError := fixture.plugin.PrepareContext(operationContext, input)
	testutil.RequireNoError(test, operationError)
	budget, operationError := input.Measure(context.Background(), selection.Request)
	testutil.RequireNoError(test, operationError)
	if budget.InputTokens >= selection.InputCeiling || fixture.view(test).Removed["current"] {
		test.Fatal("checkpoint admitted excess context or removed the protected request")
	}
	job := waitForJob(test, fixture, "applied")
	if job.Attempts != 2 || job.Failures != 0 || job.Progress == 0 {
		test.Fatalf("checkpoint did not preserve progress across worker attempts: %+v", job)
	}
	embeddings.mutex.Lock()
	defer embeddings.mutex.Unlock()
	if embeddings.firstWrites != 1 {
		test.Fatalf("retry repeated an already indexed chunk %d times", embeddings.firstWrites)
	}
}

type checkpointBlockedEmbeddings struct {
	testEmbeddings
	started chan struct{}
	once    sync.Once
}

func (embeddings *checkpointBlockedEmbeddings) Embed(operationContext context.Context, texts []string) ([][]float64, error) {
	embeddings.once.Do(func() { close(embeddings.started) })
	<-operationContext.Done()
	return nil, operationContext.Err()
}

func TestForcedCheckpointReportsTerminalWorkerFailureAfterRetries(test *testing.T) {
	fixture := newPluginFixtureWithEmbeddings(test, &checkpointBlockedEmbeddings{started: make(chan struct{})})
	messages := fixtureMessages(fixture.session, 1000)
	fixture.prepare(test, messages, 200000)
	_, operationError := fixture.plugin.UpdateConfiguration(context.Background(), fixture.session.InstanceID, json.RawMessage(`{"job_timeout_ms":1000,"worker_count":1,"retry_limit":1}`))
	testutil.RequireNoError(test, operationError)
	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, operationError = fixture.plugin.PrepareContext(operationContext, weightedContextInput(fixture.session, messages, 3000))
	if operationError == nil || !strings.Contains(operationError.Error(), "worker error: context deadline exceeded") {
		test.Fatalf("checkpoint did not report the worker's terminal failure: %v", operationError)
	}
	job := waitForJob(test, fixture, "failed")
	if job.Attempts != 2 || job.Failures != 2 || job.Progress != 0 || len(fixture.view(test).Removed) != 0 {
		test.Fatalf("stalled work exceeded its retry limit or changed source context: %+v", job)
	}
}

func TestForcedCheckpointWaitCanBeCancelled(test *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "request cancellation"
		if shutdown {
			name = "plugin shutdown"
		}
		test.Run(name, func(test *testing.T) {
			embeddings := &checkpointBlockedEmbeddings{started: make(chan struct{})}
			fixture := newPluginFixtureWithEmbeddings(test, embeddings)
			messages := fixtureMessages(fixture.session, 1000)
			fixture.prepare(test, messages, 200000)
			before := fixture.view(test)
			operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				_, operationError := fixture.plugin.PrepareContext(operationContext, weightedContextInput(fixture.session, messages, 3000))
				finished <- operationError
			}()
			select {
			case <-embeddings.started:
			case <-time.After(3 * time.Second):
				test.Fatal("checkpoint worker did not start")
			}
			if shutdown {
				fixture.plugin.cancel()
			} else {
				cancel()
			}
			select {
			case operationError := <-finished:
				if !errors.Is(operationError, context.Canceled) {
					test.Fatalf("checkpoint cancellation returned %v", operationError)
				}
			case <-time.After(time.Second):
				test.Fatal("checkpoint wait did not stop on cancellation")
			}
			testutil.RequireNoError(test, fixture.plugin.EndTurn(context.Background(), fixture.session, "cancelled"))
			waitForJob(test, fixture, "cancelled")
			if after := fixture.view(test); after.Revision != before.Revision || len(after.Removed) != 0 {
				test.Fatal("cancelled checkpoint changed source context")
			}
		})
	}
}

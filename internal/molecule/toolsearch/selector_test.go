package toolsearch

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/internal/testutil"
)

type searcherFunc func(context.Context, []Document, string) ([]Match, error)

func (searcher searcherFunc) Search(operationContext context.Context, documents []Document, query string) ([]Match, error) {
	return searcher(operationContext, documents, query)
}

func TestSelectorModesFallbackAndStrictFailure(test *testing.T) {
	backendFailure := errors.New("model unavailable")
	documents := []Document{{ID: "bash", Text: "shell command"}}
	for _, mode := range []Mode{ModeAuto, ModeSemantic, ModeLexical} {
		test.Run(string(mode), func(test *testing.T) {
			factoryCalls, reports := 0, 0
			selector, operationError := NewSelector(context.Background(), mode, func(context.Context) (Searcher, error) { factoryCalls++; return nil, backendFailure }, NewLexical(0.01), func(error) { reports++ })
			if mode == ModeSemantic {
				if !errors.Is(operationError, backendFailure) {
					test.Fatal("strict semantic mode hid initialization failure")
				}
				return
			}
			testutil.RequireNoError(test, operationError)
			defer func() { testutil.RequireNoError(test, selector.Close()) }()
			matches, operationError := selector.Search(context.Background(), documents, "shell")
			testutil.RequireNoError(test, operationError)
			if len(matches) != 1 || selector.SelectedMode() != ModeLexical {
				test.Fatalf("fallback was not usable: %v", matches)
			}
			if mode == ModeLexical && (factoryCalls != 0 || reports != 0) {
				test.Fatal("lexical mode touched the optional model")
			}
			if mode == ModeAuto && (factoryCalls != 1 || reports != 1) {
				test.Fatal("automatic initialization fallback was not reported")
			}
		})
	}
}

func TestSelectorRuntimeFallbackIsPermanent(test *testing.T) {
	var semanticCalls, lexicalCalls, reports atomic.Int32
	semantic := searcherFunc(func(context.Context, []Document, string) ([]Match, error) {
		semanticCalls.Add(1)
		return nil, errors.New("invalid vector")
	})
	lexical := searcherFunc(func(context.Context, []Document, string) ([]Match, error) {
		lexicalCalls.Add(1)
		return []Match{{ID: "bash", Similarity: 1}}, nil
	})
	selector, operationError := NewSelector(context.Background(), ModeAuto, func(context.Context) (Searcher, error) { return semantic, nil }, lexical, func(error) { reports.Add(1) })
	testutil.RequireNoError(test, operationError)
	defer func() { testutil.RequireNoError(test, selector.Close()) }()
	for range 2 {
		matches, operationError := selector.Search(context.Background(), nil, "query")
		testutil.RequireNoError(test, operationError)
		if len(matches) != 1 || matches[0].ID != "bash" {
			test.Fatal("inference failure did not fall back")
		}
	}
	if semanticCalls.Load() != 1 || lexicalCalls.Load() != 2 || reports.Load() != 1 {
		test.Fatal("failed model was retried or fallback was silent")
	}
}

func TestSelectorCancellationNeverFallsBack(test *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded} {
		var fallbackCalls atomic.Int32
		lexical := searcherFunc(func(context.Context, []Document, string) ([]Match, error) { fallbackCalls.Add(1); return nil, nil })
		selector, operationError := NewSelector(context.Background(), ModeAuto, func(context.Context) (Searcher, error) {
			return searcherFunc(func(context.Context, []Document, string) ([]Match, error) { return nil, failure }), nil
		}, lexical, func(error) { fallbackCalls.Add(1) })
		testutil.RequireNoError(test, operationError)
		_, operationError = selector.Search(context.Background(), nil, "query")
		if !errors.Is(operationError, failure) || fallbackCalls.Load() != 0 || selector.SelectedMode() != ModeSemantic {
			test.Fatal("cancellation started fallback work")
		}
		testutil.RequireNoError(test, selector.Close())
		_, operationError = NewSelector(context.Background(), ModeAuto, func(context.Context) (Searcher, error) { return nil, failure }, lexical, func(error) { fallbackCalls.Add(1) })
		if !errors.Is(operationError, failure) || fallbackCalls.Load() != 0 {
			test.Fatal("initialization cancellation started fallback work")
		}
	}
}

type ownedSearcher struct {
	entered chan struct{}
	release chan struct{}
	closes  atomic.Int32
}

func (searcher *ownedSearcher) Search(context.Context, []Document, string) ([]Match, error) {
	close(searcher.entered)
	<-searcher.release
	return nil, nil
}
func (searcher *ownedSearcher) Close() error { searcher.closes.Add(1); return nil }

func TestSelectorCloseJoinsActiveSearchAndRejectsLateWork(test *testing.T) {
	searcher := &ownedSearcher{entered: make(chan struct{}), release: make(chan struct{})}
	selector, operationError := NewSelector(context.Background(), ModeAuto, func(context.Context) (Searcher, error) { return searcher, nil }, NewLexical(0), nil)
	testutil.RequireNoError(test, operationError)
	searchDone := make(chan error, 1)
	go func() {
		_, operationError := selector.Search(context.Background(), nil, "query")
		searchDone <- operationError
	}()
	<-searcher.entered
	closeDone := make(chan error, 1)
	go func() { closeDone <- selector.Close() }()
	waitContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, operationError = selector.Search(waitContext, nil, "waiting")
	if !errors.Is(operationError, context.DeadlineExceeded) {
		test.Fatal("waiting search did not cancel")
	}
	if searcher.closes.Load() != 0 {
		test.Fatal("model closed during inference")
	}
	close(searcher.release)
	testutil.RequireNoError(test, <-searchDone)
	testutil.RequireNoError(test, <-closeDone)
	testutil.RequireNoError(test, selector.Close())
	if searcher.closes.Load() != 1 {
		test.Fatal("model resources were closed more than once")
	}
	_, operationError = selector.Search(context.Background(), nil, "late")
	if !errors.Is(operationError, ErrClosed) {
		test.Fatal("closed backend accepted late work")
	}
}

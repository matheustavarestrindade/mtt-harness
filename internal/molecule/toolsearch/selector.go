package toolsearch

import (
	"context"
	"errors"
	"fmt"
	"io"
)

var ErrClosed = errors.New("tool search is closed")

// Factory creates the optional semantic backend. Lexical mode never calls it,
// so that build needs neither a model nor an inference implementation.
type Factory func(operationContext context.Context) (Searcher, error)

// FallbackReporter reports a real initialization/inference failure that changed
// auto mode to lexical search. Cancellation is not a backend failure.
type FallbackReporter func(operationError error)

// Selector owns backend lifecycle. Its cancellable gate joins an in-flight
// search before Close releases optional model resources. Automatic fallback is
// permanent for this instance, avoiding repeated failed initialization/work.
type Selector struct {
	mode       Mode
	selected   Mode
	semantic   Searcher
	lexical    Searcher
	reporter   FallbackReporter
	gate       chan struct{}
	closed     bool
	closeError error
}

func NewSelector(operationContext context.Context, mode Mode, factory Factory, lexical Searcher, reporter FallbackReporter) (*Selector, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if lexical == nil {
		return nil, fmt.Errorf("lexical search backend is missing")
	}
	selector := &Selector{mode: mode, selected: ModeLexical, lexical: lexical, reporter: reporter, gate: make(chan struct{}, 1)}
	if mode == ModeLexical {
		return selector, nil
	}
	if mode != ModeAuto && mode != ModeSemantic {
		return nil, fmt.Errorf("unsupported tool search mode %q", mode)
	}
	if factory == nil {
		return nil, fmt.Errorf("semantic search factory is missing")
	}
	semantic, operationError := factory(operationContext)
	if operationError == nil {
		if semantic == nil {
			return nil, fmt.Errorf("semantic search factory returned no backend")
		}
		if operationError := operationContext.Err(); operationError != nil {
			if closer, available := semantic.(io.Closer); available {
				operationError = errors.Join(operationError, closer.Close())
			}
			return nil, operationError
		}
		selector.semantic, selector.selected = semantic, ModeSemantic
		return selector, nil
	}
	if mode == ModeSemantic || cancellation(operationContext, operationError) {
		return nil, operationError
	}
	selector.reportFallback(operationError)
	return selector, nil
}

// SelectedMode reports the active backend, including permanent auto fallback.
func (selector *Selector) SelectedMode() Mode {
	selector.gate <- struct{}{}
	defer func() { <-selector.gate }()
	return selector.selected
}

func (selector *Selector) Search(operationContext context.Context, documents []Document, query string) ([]Match, error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	select {
	case selector.gate <- struct{}{}:
	case <-operationContext.Done():
		return nil, operationContext.Err()
	}
	defer func() { <-selector.gate }()
	if selector.closed {
		return nil, ErrClosed
	}
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	if selector.selected == ModeLexical {
		return selector.lexical.Search(operationContext, documents, query)
	}
	matches, operationError := selector.semantic.Search(operationContext, documents, query)
	if cancellationError := operationContext.Err(); cancellationError != nil {
		return nil, cancellationError
	}
	if operationError == nil {
		return matches, nil
	}
	if selector.mode == ModeSemantic || cancellation(operationContext, operationError) {
		return nil, operationError
	}
	selector.selected = ModeLexical
	selector.reportFallback(operationError)
	return selector.lexical.Search(operationContext, documents, query)
}

// Close is irreversible and waits for owned searches. Application shutdown calls
// it after the queue joins model/tool workers, before storage is closed.
func (selector *Selector) Close() error {
	selector.gate <- struct{}{}
	defer func() { <-selector.gate }()
	if selector.closed {
		return selector.closeError
	}
	selector.closed = true
	if closer, available := selector.semantic.(io.Closer); available {
		selector.closeError = closer.Close()
	}
	return selector.closeError
}

func (selector *Selector) reportFallback(operationError error) {
	if selector.reporter != nil {
		selector.reporter(operationError)
	}
}

func cancellation(operationContext context.Context, operationError error) bool {
	return operationContext.Err() != nil || errors.Is(operationError, context.Canceled) || errors.Is(operationError, context.DeadlineExceeded)
}

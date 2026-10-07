//go:build gemma && cgo

package embeddinggemma

import (
	"context"
	"errors"
	"fmt"
	"math"

	ort "github.com/yalue/onnxruntime_go"
)

func (encoder *Encoder) runInference(operationContext context.Context, tokens []uint32) (vector []float64, operationError error) {
	if operationError := operationContext.Err(); operationError != nil {
		return nil, operationError
	}
	identifiers, mask := make([]int64, len(tokens)), make([]int64, len(tokens))
	for position, token := range tokens {
		identifiers[position], mask[position] = int64(token), 1
	}
	inputs, operationError := createInputs(identifiers, mask)
	if operationError != nil {
		return nil, operationError
	}
	defer func() { operationError = errors.Join(operationError, destroyValues(inputs)) }()
	output, operationError := ort.NewEmptyTensor[float32](ort.NewShape(1, dimensions))
	if operationError != nil {
		return nil, operationError
	}
	defer func() { operationError = errors.Join(operationError, output.Destroy()) }()
	runOptions, operationError := ort.NewRunOptions()
	if operationError != nil {
		return nil, operationError
	}
	defer func() { operationError = errors.Join(operationError, runOptions.Destroy()) }()
	termination := make(chan error, 1)
	stopCancellation := context.AfterFunc(operationContext, func() { termination <- runOptions.Terminate() })
	operationError = encoder.session.RunWithOptions(inputs, []ort.Value{output}, runOptions)
	// A native cancellation callback must finish before its options or tensors
	// are freed. The next call uses new options and cannot inherit termination.
	if !stopCancellation() {
		operationError = errors.Join(operationError, <-termination)
	}
	if cancellationError := operationContext.Err(); cancellationError != nil {
		return nil, errors.Join(cancellationError, operationError)
	}
	if operationError != nil {
		return nil, operationError
	}
	data := output.GetData()
	if len(data) != dimensions {
		return nil, fmt.Errorf("EmbeddingGemma returned %d dimensions; expected %d", len(data), dimensions)
	}
	vector = make([]float64, dimensions)
	var squaredNorm float64
	for position, value := range data {
		converted := float64(value)
		if math.IsNaN(converted) || math.IsInf(converted, 0) {
			return nil, fmt.Errorf("EmbeddingGemma returned a non-finite vector")
		}
		vector[position] = converted
		squaredNorm += converted * converted
	}
	if squaredNorm == 0 {
		return nil, fmt.Errorf("EmbeddingGemma returned a zero vector")
	}
	norm := math.Sqrt(squaredNorm)
	for position := range vector {
		vector[position] /= norm
	}
	return vector, nil
}

func createInputs(identifiers, mask []int64) (values []ort.Value, operationError error) {
	defer func() {
		if operationError != nil {
			operationError = errors.Join(operationError, destroyValues(values))
			values = nil
		}
	}()
	for _, data := range [][]int64{identifiers, mask} {
		tensor, createError := ort.NewTensor(ort.NewShape(1, int64(len(data))), data)
		if createError != nil {
			return values, createError
		}
		values = append(values, tensor)
	}
	for range 3 {
		empty, createError := ort.NewEmptyTensor[float32](ort.NewShape(0, featureDimensions))
		if createError != nil {
			return values, createError
		}
		values = append(values, empty)
	}
	return values, nil
}

func destroyValues(values []ort.Value) error {
	var operationError error
	for _, value := range values {
		if value != nil {
			operationError = errors.Join(operationError, value.Destroy())
		}
	}
	return operationError
}

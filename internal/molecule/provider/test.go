package provider

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Test struct {
	providerName string
	model        atom.ModelInfo
	mutex        sync.Mutex
	script       [][]atom.ResponsePart
	index        int
	delay        time.Duration
	Requests     []atom.Request
}

func NewTest(name string, script ...[]atom.ResponsePart) *Test {
	if name == "" {
		name = "test"
	}
	return &Test{
		providerName: name,
		model: atom.ModelInfo{
			ID:         name + "-model",
			Input:      []atom.MediaType{atom.Text},
			Output:     []atom.MediaType{atom.Text},
			Tools:      true,
			ContextMax: 128000,
			Prices:     &atom.Prices{Currency: "USD", Input: 1, Output: 2},
		},
		script: script,
	}
}

func (testProvider *Test) Name() string {
	return testProvider.providerName
}

func (testProvider *Test) Models() []atom.ModelInfo {
	return []atom.ModelInfo{testProvider.model}
}

func (testProvider *Test) Stream(operationContext context.Context, request atom.Request) (harness.Stream, error) {
	testProvider.mutex.Lock()
	defer testProvider.mutex.Unlock()
	testProvider.Requests = append(testProvider.Requests, request)
	if testProvider.index >= len(testProvider.script) {
		return &sliceStream{parts: Text("done")}, nil
	}
	parts := testProvider.script[testProvider.index]
	testProvider.index++
	return &sliceStream{parts: parts, delay: testProvider.delay}, nil
}

func (testProvider *Test) Calls() int {
	testProvider.mutex.Lock()
	defer testProvider.mutex.Unlock()
	return testProvider.index
}

type sliceStream struct {
	parts []atom.ResponsePart
	index int
	delay time.Duration
}

func (testProvider *Test) SetDelay(delay time.Duration) {
	testProvider.mutex.Lock()
	defer testProvider.mutex.Unlock()
	testProvider.delay = delay
}

func (responseStream *sliceStream) Recv(operationContext context.Context) (atom.ResponsePart, error) {
	if responseStream.delay > 0 {
		select {
		case <-time.After(responseStream.delay):
			responseStream.delay = 0
		case <-operationContext.Done():
			return atom.ResponsePart{}, operationContext.Err()
		}
	}
	if responseStream.index >= len(responseStream.parts) {
		return atom.ResponsePart{}, io.EOF
	}
	part := responseStream.parts[responseStream.index]
	responseStream.index++
	return part, nil
}

func Text(text string) []atom.ResponsePart {
	return []atom.ResponsePart{{Text: text}}
}

func Call(name string, input string) []atom.ResponsePart {
	return []atom.ResponsePart{{
		ToolCall: &atom.ToolCall{ID: "call_" + name, Name: name, Input: []byte(input)},
	}}
}

func CallWithUsage(name string, input string, usage atom.Usage) []atom.ResponsePart {
	return []atom.ResponsePart{
		{ToolCall: &atom.ToolCall{ID: "call_" + name, Name: name, Input: []byte(input)}},
		{Usage: &usage},
	}
}

func TextWithUsage(text string, usage atom.Usage) []atom.ResponsePart {
	return []atom.ResponsePart{
		{Text: text},
		{Usage: &usage},
	}
}

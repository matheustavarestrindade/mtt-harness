package provider

import (
	"context"
	"io"
	"sync"

	"github.com/matheustavarestrindade/mtt-harness/atom"
	"github.com/matheustavarestrindade/mtt-harness/harness"
)

type Test struct {
	providerName string
	model        atom.ModelInfo
	mu           sync.Mutex
	script       [][]atom.ResponsePart
	index        int
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

func (t *Test) Name() string {
	return t.providerName
}

func (t *Test) Models() []atom.ModelInfo {
	return []atom.ModelInfo{t.model}
}

func (t *Test) Stream(ctx context.Context, request atom.Request) (harness.Stream, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Requests = append(t.Requests, request)
	if t.index >= len(t.script) {
		return &sliceStream{parts: Text("done")}, nil
	}
	parts := t.script[t.index]
	t.index++
	return &sliceStream{parts: parts}, nil
}

func (t *Test) Calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.index
}

type sliceStream struct {
	parts []atom.ResponsePart
	index int
}

func (s *sliceStream) Recv(ctx context.Context) (atom.ResponsePart, error) {
	if s.index >= len(s.parts) {
		return atom.ResponsePart{}, io.EOF
	}
	part := s.parts[s.index]
	s.index++
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

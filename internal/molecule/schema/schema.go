package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Validate implements JSON Schema rather than silently ignoring unsupported
// keywords. References inside the supplied schema work; remote references must
// be bundled by the tool owner and cannot trigger network or file reads here.
func Validate(raw []byte, input json.RawMessage) error {
	if len(raw) == 0 {
		raw = []byte(`{"type":"object"}`)
	}
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(url string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema reference %q is not bundled", url)
	}
	if operationError := compiler.AddResource("https://mtt.invalid/tool.json", bytes.NewReader(raw)); operationError != nil {
		return fmt.Errorf("invalid tool schema: %w", operationError)
	}
	compiled, operationError := compiler.Compile("https://mtt.invalid/tool.json")
	if operationError != nil {
		return fmt.Errorf("invalid tool schema: %w", operationError)
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value any
	if operationError := decoder.Decode(&value); operationError != nil {
		return operationError
	}
	var extra any
	if operationError := decoder.Decode(&extra); operationError != io.EOF {
		return fmt.Errorf("tool input must contain one JSON value")
	}
	return compiled.Validate(value)
}

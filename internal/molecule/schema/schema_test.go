package schema

import "testing"

const example = `{
	"type": "object",
	"properties": {
		"path": {"type": "string"},
		"limit": {"type": "integer"},
		"tags": {"type": "array", "items": {"type": "string"}}
	},
	"required": ["path"]
}`

func TestValidate(test *testing.T) {
	cases := []struct {
		name  string
		input string
		found bool
	}{
		{"correct", `{"path": "/tmp", "limit": 3, "tags": ["a", "b"]}`, true},
		{"extra field", `{"path": "/tmp", "other": true}`, true},
		{"missing field", `{"limit": 3}`, false},
		{"bad string", `{"path": 5}`, false},
		{"bad integer", `{"path": "/tmp", "limit": "many"}`, false},
		{"bad array item", `{"path": "/tmp", "tags": [1]}`, false},
		{"not an object", `["x"]`, false},
		{"empty input", ``, false},
	}
	for _, item := range cases {
		operationError := Validate([]byte(example), []byte(item.input))
		if item.found && operationError != nil {
			test.Fatalf("%s: %v", item.name, operationError)
		}
		if !item.found && operationError == nil {
			test.Fatalf("%s: the error is absent", item.name)
		}
	}
	if operationError := Validate(nil, []byte(`{}`)); operationError != nil {
		test.Fatalf("empty schema: %v", operationError)
	}
	if operationError := Validate(nil, []byte(`anything`)); operationError == nil {
		test.Fatal("an absent schema must not admit invalid JSON")
	}
}

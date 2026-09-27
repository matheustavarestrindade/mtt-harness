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

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		input string
		ok    bool
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
		err := Validate([]byte(example), []byte(item.input))
		if item.ok && err != nil {
			t.Fatalf("%s: %v", item.name, err)
		}
		if !item.ok && err == nil {
			t.Fatalf("%s: the error is absent", item.name)
		}
	}
	if err := Validate(nil, []byte(`anything`)); err != nil {
		t.Fatalf("empty schema: %v", err)
	}
}

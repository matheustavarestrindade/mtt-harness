package schema

import "testing"

func TestSchemaConstraintsAreEnforced(test *testing.T) {
	for _, scenario := range []struct {
		name, definition, input string
		valid                   bool
	}{
		{"null", `{"type":"object","required":["path"],"properties":{"path":{"type":"string"}}}`, `{"path":null}`, false},
		{"enum", `{"type":"object","properties":{"mode":{"enum":["exit","error"]}}}`, `{"mode":"invalid"}`, false},
		{"extras", `{"type":"object","additionalProperties":false}`, `{"extra":true}`, false},
		{"reference", `{"$defs":{"positive":{"type":"integer","minimum":1}},"type":"object","properties":{"count":{"$ref":"#/$defs/positive"}}}`, `{"count":0}`, false},
		{"reference valid", `{"$defs":{"positive":{"type":"integer","minimum":1}},"type":"object","properties":{"count":{"$ref":"#/$defs/positive"}}}`, `{"count":3}`, true},
		{"union", `{"type":["string","null"]}`, `null`, true},
		{"bad schema", `{"type":42}`, `{}`, false},
		{"remote reference", `{"$ref":"file:///etc/passwd"}`, `{}`, false},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			operationError := Validate([]byte(scenario.definition), []byte(scenario.input))
			if (operationError == nil) != scenario.valid {
				test.Fatalf("valid=%t: %v", scenario.valid, operationError)
			}
		})
	}
}

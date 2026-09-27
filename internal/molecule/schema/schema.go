package schema

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type definition struct {
	Type       string                `json:"type"`
	Required   []string              `json:"required"`
	Properties map[string]definition `json:"properties"`
	Items      *definition           `json:"items"`
}

func Validate(raw []byte, input json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var definition definition
	if err := json.Unmarshal(raw, &definition); err != nil {
		return nil
	}
	if definition.Type != "object" {
		return nil
	}
	data := input
	if len(data) == 0 {
		data = json.RawMessage("{}")
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return errors.New("the input is not a JSON object")
	}
	return validateObject(definition, value, "")
}

func validateObject(definition definition, value map[string]json.RawMessage, path string) error {
	for _, name := range definition.Required {
		if _, ok := value[name]; !ok {
			return fmt.Errorf("the field %s is necessary", fieldPath(path, name))
		}
	}
	for name, item := range value {
		property, ok := definition.Properties[name]
		if !ok || property.Type == "" {
			continue
		}
		if err := validateValue(property, item, fieldPath(path, name)); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(definition definition, data json.RawMessage, path string) error {
	switch definition.Type {
	case "string":
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not a string", path)
		}
	case "integer":
		var value int64
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not an integer", path)
		}
	case "number":
		var value float64
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not a number", path)
		}
	case "boolean":
		var value bool
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not a boolean", path)
		}
	case "object":
		var value map[string]json.RawMessage
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not an object", path)
		}
		return validateObject(definition, value, path)
	case "array":
		var value []json.RawMessage
		if err := json.Unmarshal(data, &value); err != nil {
			return fmt.Errorf("the field %s is not an array", path)
		}
		if definition.Items != nil {
			for index, item := range value {
				if err := validateValue(*definition.Items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func fieldPath(path string, name string) string {
	if path == "" {
		return name
	}
	if strings.HasSuffix(path, "]") {
		return path + "." + name
	}
	return path + "." + name
}

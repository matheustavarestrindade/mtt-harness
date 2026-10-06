package taskstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

func taskUpdateObject(data []byte, allowed []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if operationError := json.Unmarshal(data, &fields); operationError != nil {
		return nil, operationError
	}
	if fields == nil {
		return nil, fmt.Errorf("task state input must be an object")
	}
	for name := range fields {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("unknown task state field %q", name)
		}
	}
	return fields, nil
}

// DecodeUpdate preserves field presence: null is meaningful only for doing.
func DecodeUpdate(data []byte) (atom.TaskStateUpdate, error) {
	update := atom.TaskStateUpdate{}
	fields, operationError := taskUpdateObject(data, []string{"todo", "doing"})
	if operationError != nil {
		return update, operationError
	}
	if encoded, present := fields["todo"]; present {
		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return update, fmt.Errorf("todo cannot be null")
		}
		var items []json.RawMessage
		if operationError := json.Unmarshal(encoded, &items); operationError != nil {
			return update, operationError
		}
		updates := make([]atom.TaskItemUpdate, 0, len(items))
		for _, encodedItem := range items {
			itemFields, operationError := taskUpdateObject(encodedItem, []string{"id", "title", "status"})
			if operationError != nil {
				return update, operationError
			}
			for name, value := range itemFields {
				if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return update, fmt.Errorf("todo %s cannot be null", name)
				}
			}
			var item atom.TaskItemUpdate
			if operationError := json.Unmarshal(encodedItem, &item); operationError != nil {
				return update, operationError
			}
			updates = append(updates, item)
		}
		update.Todo = &updates
	}
	if encoded, present := fields["doing"]; present {
		update.DoingProvided = true
		if !bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			if _, operationError := taskUpdateObject(encoded, []string{"title", "description"}); operationError != nil {
				return update, operationError
			}
			var doing atom.DoingState
			if operationError := json.Unmarshal(encoded, &doing); operationError != nil {
				return update, operationError
			}
			update.Doing = &doing
		}
	}
	return update, ValidateUpdate(update)
}

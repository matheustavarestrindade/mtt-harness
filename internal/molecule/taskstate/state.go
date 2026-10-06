package taskstate

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/matheustavarestrindade/mtt-harness/atom"
)

const MaximumItems = 32
const MaximumItemTitle = 160
const MaximumDoingTitle = 120
const MaximumDoingDescription = 1200
const ReminderResponses = 3

func Empty(sessionID atom.SessionID) atom.TaskState {
	return atom.TaskState{SessionID: sessionID, Todo: []atom.TaskItem{}}
}

func Active(state atom.TaskState) bool { return len(state.Todo) > 0 || state.Doing != nil }

func Clone(state atom.TaskState) atom.TaskState {
	state.Todo = append([]atom.TaskItem{}, state.Todo...)
	if state.Doing != nil {
		doing := *state.Doing
		state.Doing = &doing
	}
	return state
}

func validTaskStatus(status atom.TaskStatus) bool {
	return status == atom.TaskPending || status == atom.TaskInProgress || status == atom.TaskDone || status == atom.TaskCancelled
}

func validateTaskText(name, text string, limit int) error {
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > limit {
		return fmt.Errorf("%s must contain 1-%d characters of non-blank text", name, limit)
	}
	return nil
}

func ValidateUpdate(update atom.TaskStateUpdate) error {
	if update.Todo == nil && !update.DoingProvided {
		return fmt.Errorf("todo or doing is required")
	}
	if update.Todo != nil {
		if len(*update.Todo) > MaximumItems {
			return fmt.Errorf("todo exceeds %d items", MaximumItems)
		}
		seen := make(map[string]bool, len(*update.Todo))
		for _, item := range *update.Todo {
			if item.ID == "" || len(item.ID) > 64 {
				return fmt.Errorf("todo ID must contain 1-64 ASCII letters, digits, hyphens or underscores")
			}
			for _, character := range item.ID {
				if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_') {
					return fmt.Errorf("invalid todo ID %q", item.ID)
				}
			}
			if seen[item.ID] {
				return fmt.Errorf("duplicate todo ID %q", item.ID)
			}
			seen[item.ID] = true
			if item.Title != nil {
				if operationError := validateTaskText("todo title", *item.Title, MaximumItemTitle); operationError != nil {
					return operationError
				}
			}
			if item.Status != nil && !validTaskStatus(*item.Status) {
				return fmt.Errorf("invalid todo status %q", *item.Status)
			}
		}
	}
	if update.DoingProvided && update.Doing != nil {
		if operationError := validateTaskText("doing title", update.Doing.Title, MaximumDoingTitle); operationError != nil {
			return operationError
		}
		if operationError := validateTaskText("doing description", update.Doing.Description, MaximumDoingDescription); operationError != nil {
			return operationError
		}
	}
	return nil
}

// ApplyUpdate merges items by stable ID while retaining their order. Reaffirming
// unchanged state still records a review and resets reminder freshness. Once all
// listed work is terminal, both fields clear in the same stored revision.
func ApplyUpdate(current atom.TaskState, update atom.TaskStateUpdate, now time.Time) (atom.TaskState, error) {
	if operationError := ValidateUpdate(update); operationError != nil {
		return atom.TaskState{}, operationError
	}
	next := Clone(current)
	if update.Todo != nil {
		if len(*update.Todo) == 0 {
			next.Todo = []atom.TaskItem{}
			next.Doing = nil
		}
		positions := make(map[string]int, len(next.Todo))
		for index, item := range next.Todo {
			positions[item.ID] = index
		}
		for _, item := range *update.Todo {
			position, found := positions[item.ID]
			if !found {
				if item.Title == nil {
					return atom.TaskState{}, fmt.Errorf("title is required for new todo %q", item.ID)
				}
				position = len(next.Todo)
				positions[item.ID] = position
				next.Todo = append(next.Todo, atom.TaskItem{ID: item.ID, Status: atom.TaskPending})
			}
			if item.Title != nil {
				next.Todo[position].Title = *item.Title
			}
			if item.Status != nil {
				next.Todo[position].Status = *item.Status
			}
		}
	}
	if len(next.Todo) > MaximumItems {
		return atom.TaskState{}, fmt.Errorf("todo exceeds %d stored items", MaximumItems)
	}
	if update.DoingProvided {
		next.Doing = update.Doing
	}
	allTerminal := len(next.Todo) > 0
	for _, item := range next.Todo {
		if item.Status != atom.TaskDone && item.Status != atom.TaskCancelled {
			allTerminal = false
			break
		}
	}
	if allTerminal {
		next.Todo = []atom.TaskItem{}
		next.Doing = nil
	}
	next.Revision++
	next.UpdatedAt = now.UTC()
	next.ResponsesSinceUpdate = 0
	return Clone(next), nil
}

func Clear(current atom.TaskState, now time.Time) atom.TaskState {
	current.Todo = []atom.TaskItem{}
	current.Doing = nil
	current.Revision++
	current.UpdatedAt = now.UTC()
	current.ResponsesSinceUpdate = 0
	return current
}

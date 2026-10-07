package contextplugin

import (
	"strconv"
	"time"
)

// invalidateIdeas walks all ancestors, including ideas already consolidated
// into another idea. Existing details become visible while repairs are pending.
func invalidateIdeas(database transaction, memories []memoryRecord, changedID, replacementID string) error {
	changed := map[string]string{changedID: replacementID}
	visited := map[string]bool{}
	for {
		advanced := false
		for _, candidate := range memories {
			if candidate.Kind != "idea" || visited[candidate.ID] {
				continue
			}
			idea, operationError := readTransactionValue[memoryRecord](database, "memory", candidate.ID)
			if operationError != nil {
				return operationError
			}
			if idea.Status != "active" && idea.Status != "consolidated" {
				continue
			}
			needsRepair := false
			for index, child := range idea.Children {
				if replacement, found := changed[child]; found {
					idea.Children[index] = replacement
					needsRepair = true
				}
			}
			if !needsRepair {
				continue
			}
			visited[idea.ID] = true
			changed[idea.ID] = idea.ID
			advanced = true
			if operationError := activateSupportedChildren(database, idea.Children); operationError != nil {
				return operationError
			}
			idea.Status = "invalidated"
			if operationError := database.Put("memory", idea.ID, idea); operationError != nil {
				return operationError
			}
			if operationError := database.MarkVectors("memory", idea.ID, false, true); operationError != nil {
				return operationError
			}
			job := memoryJob{ID: "repair-" + idea.ID + "-" + strconv.Itoa(idea.Version), Kind: "repair", Agent: "context.historian", Status: "pending", Priority: 5, MemoryIDs: idea.Children, ResultIDs: []string{idea.ID}, CreatedAt: time.Now().UTC()}
			previous, operationError := readTransactionValue[memoryJob](database, "job", job.ID)
			if operationError != nil {
				return operationError
			}
			if previous.ID != "" {
				job.Attempts = previous.Attempts
			}
			if operationError := database.Put("job", job.ID, job); operationError != nil {
				return operationError
			}
		}
		if !advanced {
			return nil
		}
	}
}

func activateSupportedChildren(database transaction, children []string) error {
	for _, identifier := range children {
		child, operationError := readTransactionValue[memoryRecord](database, "memory", identifier)
		if operationError != nil {
			return operationError
		}
		if child.Status != "consolidated" {
			continue
		}
		child.Status = "active"
		if operationError := database.Put("memory", identifier, child); operationError != nil {
			return operationError
		}
	}
	return nil
}

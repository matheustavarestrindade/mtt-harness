package store

import "fmt"

// ValidateQueuedBatch checks admission bounds and unique IDs before storage changes.
func ValidateQueuedBatch(messageIDs []string) error {
	if len(messageIDs) == 0 || len(messageIDs) > 128 {
		return fmt.Errorf("queued batch must contain 1 to 128 messages")
	}
	seen := make(map[string]bool, len(messageIDs))
	for _, identifier := range messageIDs {
		if identifier == "" || seen[identifier] {
			return fmt.Errorf("queued batch contains an empty or duplicate message ID")
		}
		seen[identifier] = true
	}
	return nil
}

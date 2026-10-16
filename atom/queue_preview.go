package atom

import "time"

// QueuedMessagePreview is a bounded, read-only projection of pending input.
// Truncated means some text, content parts, or media bytes require a full read.
// The original queued message is never changed by this projection.
type QueuedMessagePreview struct {
	ID        string
	SessionID SessionID
	Role      Role
	Content   []Content
	CreatedAt time.Time
	Truncated bool
}

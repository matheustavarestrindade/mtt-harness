package atom

// LiveResponse is a bounded display snapshot, never provider input or saved
// conversation history. Revision deduplicates streamed chunks after reconnect.
type LiveResponse struct {
	Message   Message
	Model     string
	Revision  uint64
	Truncated bool
}

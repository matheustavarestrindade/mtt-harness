package atom

// QueuedMessage is persisted before the API acknowledges acceptance. Running
// entries survive crashes so recovery can report interruption without replaying
// external side effects from an uncertain turn.
type QueuedMessage struct {
	Message  Message
	Running  bool
	Sequence uint64
}

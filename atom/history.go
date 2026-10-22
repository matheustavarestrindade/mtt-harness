package atom

import "time"

// ResponseUsageSnapshot describes the latest saved assistant response, including
// an unknown report. It is independent of the currently loaded history window.
type ResponseUsageSnapshot struct {
	MessageID string
	Seq       int64
	Model     string
	Usage     *Usage
	CreatedAt time.Time
}

// HistoryPage is a bounded public history view. Context contains only the tool
// owner needed at a cut page boundary; loading older history restores its prose.
// It never includes private provider continuation state.
type HistoryPage struct {
	SessionID      SessionID
	Revision       uint64
	Messages       []Message
	Context        []Message
	Models         map[string]string
	Sizes          map[string]int
	FirstSeq       int64
	LastSeq        int64
	HeadSeq        int64
	HasOlder       bool
	HasNewer       bool
	LatestResponse *ResponseUsageSnapshot
	Processes      []ProcessRecord
	Task           string `json:",omitempty"`
	// TaskMessageID and TaskSeq identify the first saved user input of a child
	// session. Task remains a bounded label. Use Revision and a one-message
	// history read before TaskSeq+1 to retrieve the exact public input.
	TaskMessageID string `json:",omitempty"`
	TaskSeq       int64  `json:",omitempty"`
}

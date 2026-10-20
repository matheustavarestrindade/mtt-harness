package atom

import "time"

// UsageQuery selects recorded usage within inclusive UTC calendar dates.
// A session includes its conversation tree; workspace/harness scope also
// includes background workers. AgentPrefix selects workspace worker names.
type UsageQuery struct {
	From        time.Time
	To          time.Time
	InstanceID  string
	SessionID   SessionID
	AgentPrefix string
}

// UsageBucket contains one UTC day/model aggregate. PricedCalls counts records
// with an explicit cost, including zero. Missing costs stay absent; currencies
// and estimated costs retain their original meaning in Statistics.
type UsageBucket struct {
	Date        string
	ModelID     string
	Statistics  Statistics
	PricedCalls int
	FailedCalls int
}

// UsageReport is a bounded, complete aggregation, never sampled UI history.
type UsageReport struct {
	From        string
	To          string
	Timezone    string
	Statistics  Statistics
	PricedCalls int
	FailedCalls int
	Buckets     []UsageBucket
}

package atom

import (
	"encoding/json"
	"time"
)

type Prices struct {
	Currency   string
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
}

type Cost struct {
	Currency string
	Value    float64
}

type ModelInfo struct {
	ID         string
	Level      int
	Input      []MediaType
	Output     []MediaType
	Tools      bool
	ContextMax int
	Prices     *Prices
}

type ProviderSpec struct {
	Name          string
	APIURL        string
	ModelListURL  string
	PriceTableURL string
	Interval      time.Duration
}

type ProviderKey struct {
	Scope    string
	Provider string
	Key      string
}

type Usage struct {
	Input      int
	CacheRead  int
	CacheWrite int
	Output     int
	Reasoning  int
	Cost       *Cost
}

type UsageRecord struct {
	InstanceID string
	SessionID  SessionID
	ModelID    string
	Usage      Usage
	CreatedAt  time.Time
}

type Statistics struct {
	Calls      int
	Input      int
	CacheRead  int
	CacheWrite int
	Output     int
	Reasoning  int
	Cost       *Cost
	Costs      []Cost
}

func (statistics Statistics) CacheHitRate() float64 {
	full := statistics.Input + statistics.CacheRead + statistics.CacheWrite
	if full == 0 {
		return 0
	}
	return float64(statistics.CacheRead) / float64(full)
}

func (statistics Statistics) MarshalJSON() ([]byte, error) {
	type fields Statistics
	return json.Marshal(struct {
		fields
		CacheHitRate       float64
		CacheHitPercentage float64
	}{fields: fields(statistics), CacheHitRate: statistics.CacheHitRate(), CacheHitPercentage: statistics.CacheHitRate() * 100})
}

type Request struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
	Params   map[string]any
}

type ResponsePart struct {
	Text      string
	Content   []Content
	ToolIndex *int
	ToolCall  *ToolCall
	Usage     *Usage
}

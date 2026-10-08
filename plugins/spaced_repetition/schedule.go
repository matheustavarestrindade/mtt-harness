package spacedrepetition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
)

type checkpoint struct {
	level   string
	crossed int64
	next    int
	cursor  int64
}

func intervalTokens(configuration configuration, contextLimit int) int {
	if configuration.Interval.Mode == "tokens" {
		return configuration.Interval.Tokens
	}
	if contextLimit <= 0 {
		return 0
	}
	return max(256, min(configuration.Interval.MaxTokens, int(math.Floor(float64(contextLimit)*configuration.Interval.Fraction))))
}

func scheduleSignature(configuration configuration, promptVersion string) string {
	data, _ := json.Marshal(struct {
		Interval      intervalConfiguration
		Pattern       []string
		PromptVersion string
	}{configuration.Interval, configuration.Pattern, promptVersion})
	fingerprint := sha256.Sum256(data)
	return hex.EncodeToString(fingerprint[:])
}

// observeGrowth compares retained message identities, not token-count noise, to
// identify real context removal. Compaction and model changes preserve phase.
func observeGrowth(state *scheduleState, configuration configuration, model string, contextLimit, tokens int, visible []string, promptVersion string) checkpoint {
	step := intervalTokens(configuration, contextLimit)
	signature := scheduleSignature(configuration, promptVersion)
	retained := make(map[string]bool, len(visible))
	for _, identifier := range visible {
		retained[identifier] = true
	}
	removed := false
	for _, identifier := range state.Visible {
		if !retained[identifier] {
			removed = true
			break
		}
	}
	changed := state.Initialized && (state.Model != model || state.ContextLimit != contextLimit || state.Step != step || state.Signature != signature)
	if !state.Initialized {
		state.Initialized = true
		state.Next = step
	}
	if removed || changed {
		state.HighWater = tokens
		if step > 0 {
			state.Next = (tokens/step + 1) * step
		} else {
			state.Next = 0
		}
	}
	state.Model = model
	state.ContextLimit = contextLimit
	state.Step = step
	state.Signature = signature
	state.Visible = append([]string(nil), visible...)
	state.HighWater = max(state.HighWater, tokens)
	if step == 0 || tokens < state.Next {
		return checkpoint{}
	}
	crossed := int64((tokens-state.Next)/step + 1)
	level := "low"
	for position := int64(0); position < min(crossed, int64(len(configuration.Pattern))); position++ {
		if configuration.Pattern[(state.Cursor+position)%int64(len(configuration.Pattern))] == "medium" {
			level = "medium"
			break
		}
	}
	return checkpoint{level: level, crossed: crossed, next: state.Next + int(crossed)*step, cursor: state.Cursor + crossed}
}

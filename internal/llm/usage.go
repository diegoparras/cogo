package llm

import "sync"

// TokenUsage is COGO's running tally of tokens spent on the OPTIONAL model
// (contradiction lint + guard analysis). It is process-global on purpose:
// providers are built ad hoc all over the code, so every Complete call funnels
// its usage through here instead of threading a meter around.
type TokenUsage struct {
	Prompt     int64 `json:"prompt"`
	Completion int64 `json:"completion"`
	Total      int64 `json:"total"`
	Calls      int64 `json:"calls"`
}

var (
	meterMu sync.Mutex
	meter   TokenUsage
)

// addUsage records one model call. When the endpoint reports a usage block we
// trust it (exact); otherwise the caller passes a chars/4 estimate so the
// counter is approximate but never blind.
func addUsage(prompt, completion, total int) {
	meterMu.Lock()
	defer meterMu.Unlock()
	if total <= 0 {
		total = prompt + completion
	}
	meter.Prompt += int64(prompt)
	meter.Completion += int64(completion)
	meter.Total += int64(total)
	meter.Calls++
}

// Usage returns the running tally (cumulative, including any seeded baseline).
func Usage() TokenUsage {
	meterMu.Lock()
	defer meterMu.Unlock()
	return meter
}

// SeedUsage sets the tally to a persisted baseline so the count survives
// restarts. Call once at startup, before serving.
func SeedUsage(u TokenUsage) {
	meterMu.Lock()
	meter = u
	meterMu.Unlock()
}

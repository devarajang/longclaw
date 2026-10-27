package domain

// StressTestResults summarises one stress test's request_response_log rows
type StressTestResults struct {
	StressTest
	RequestSummary
	AchievedPerSecond float64                   `json:"achieved_per_second"` // logged / seconds run so far (at most test_time_secs), across all connections
	FollowUpsNotSent  int                       `json:"follow_ups_not_sent"` // reversals/advices waiting for, or never getting, their original's response
	ByMTI             map[string]RequestSummary `json:"by_mti"`              // originals and follow-ups (e.g. 0100, 0420) separately
}

// RequestSummary counts requests and their response times
type RequestSummary struct {
	Logged     int            `json:"logged"`     // requests written to a socket and logged
	Answered   int            `json:"answered"`   // requests with a matched response
	Unanswered int            `json:"unanswered"` // logged minus answered (includes in-flight while running)
	LatencyMs  LatencySummary `json:"latency_ms"` // over answered requests only
}

// LatencySummary holds response-time statistics in milliseconds
type LatencySummary struct {
	Min float64 `json:"min"`
	Avg float64 `json:"avg"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	P99 float64 `json:"p99"`
	Max float64 `json:"max"`
}

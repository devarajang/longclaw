package domain

import "time"

// Node test modes: how request_per_second is applied when several nodes (client connections) are connected
const (
	NodeTestModeAdvertised = "advertised"  // every node receives request_per_second (default)
	NodeTestModeRoundRobin = "round_robin" // request_per_second in total, sent to one node at a time in turn
)

// StressTest represents a stress test configuration
type StressTest struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	CreatedAt        time.Time `json:"created_at"`
	TotalRequests    int       `json:"total_requests"`
	TestTimeSecs     int       `json:"test_time_secs"`
	RequestPerSecond int       `json:"request_per_second"`
	NodeTestMode     string    `json:"node_test_mode"` // NodeTestModeAdvertised or NodeTestModeRoundRobin
}

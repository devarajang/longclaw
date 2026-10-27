package domain

import "time"

const (
	TestTypeSale   = "sale"
	TestTypeRefund = "refund"
)

// StressTest represents a stress test configuration
type StressTest struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	CreatedAt        time.Time `json:"created_at"`
	TotalRequests    int       `json:"total_requests"`
	TestTimeSecs     int       `json:"test_time_secs"`
	RequestPerSecond int       `json:"request_per_second"`
	TestType         string    `json:"test_type"`      // sale (default) or refund
	SourceTestID     int       `json:"source_test_id"` // refund tests: test whose sales get refunded
}

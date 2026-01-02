package domain

import "time"

// RequestLog represents a single request-response log entry
type RequestLog struct {
	ID           int       `json:"id"`
	Reference    string    `json:"reference"`
	ConnectionID string    `json:"connection_id"`
	RequestTime  time.Time `json:"request_time"`
	ResponseTime time.Time `json:"response_time"`
	TimeTaken    int       `json:"time_taken"` // in milliseconds
	CreatedAt    time.Time `json:"created_at"`
	StressTestID int       `json:"stresstest_id"`
}

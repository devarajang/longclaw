package domain

import "time"

// RequestLog represents a single request-response log entry
type RequestLog struct {
	ID           int       `json:"id"`
	Reference    string    `json:"reference"`
	ConnectionID string    `json:"connection_id"`
	MTI          string    `json:"mti"` // request MTI; empty for rows logged before the column existed
	RequestTime  time.Time `json:"request_time"`
	ResponseTime time.Time `json:"response_time"`
	TimeTaken    int       `json:"time_taken"` // in milliseconds
	CreatedAt    time.Time `json:"created_at"`
	StressTestID int       `json:"stresstest_id"`
}

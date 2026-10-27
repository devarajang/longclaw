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

	// Request details, so a later refund test can find and replay approved sales
	ProcessingCode string `json:"processing_code"`
	PAN            string `json:"pan"`
	Amount         string `json:"amount"`
	Currency       string `json:"currency"`
	RequestMessage string `json:"-"`
	ResponseCode   string `json:"response_code"`
	AuthID         string `json:"auth_id"`
	OriginalLogID  int    `json:"original_log_id"` // refund rows: id of the sale being refunded
}

// RefundCandidate is an approved sale that has not been reversed or refunded yet
type RefundCandidate struct {
	LogID          int
	Reference      string
	RequestMessage string
}

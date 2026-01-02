package domain

import "time"

type ScheduledMessage struct {
	ID           int       `json:"id"`
	Reference    string    `json:"reference"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
	ConnectionID string    `json:"connection_id"`
	StressTestID int       `json:"stresstest_id"`
	SentAt       time.Time `json:"sent_at"`
}

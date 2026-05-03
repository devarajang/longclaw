package database

import (
	"fmt"
	"time"

	"github.com/devarajang/longclaw/internal/domain"
)

// Create a new stress test
func (s *StressTestDB) CreateStressTest(name string, testTimeSecs, requestsPerSecond int) (*domain.StressTest, error) {
	result, err := s.db.Exec(
		"INSERT INTO stress_test (name, test_time_secs, request_per_second) VALUES (?, ?, ?)",
		name, testTimeSecs, requestsPerSecond,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &domain.StressTest{
		ID:               int(id),
		Name:             name,
		CreatedAt:        time.Now(),
		TestTimeSecs:     testTimeSecs,
		RequestPerSecond: requestsPerSecond,
	}, nil
}

func (s *StressTestDB) UpdateResponseTime(reference string, connectionID string) error {
	// First, get the request_time
	var requestTime time.Time
	err := withSQLiteBusyRetry(func() error {
		return s.db.QueryRow(
			"SELECT request_time FROM request_response_log WHERE reference = ? AND connection_id = ?",
			reference, connectionID,
		).Scan(&requestTime)
	})
	if err != nil {
		return fmt.Errorf("failed to get request time: %w", err)
	}

	// Calculate time taken in nanoseconds
	responseTime := time.Now()
	timeTaken := responseTime.Sub(requestTime).Milliseconds()

	// Update with calculated value
	err = withSQLiteBusyRetry(func() error {
		_, execErr := s.db.Exec(
			"UPDATE request_response_log SET response_time = ?, time_taken = ? WHERE reference = ? AND connection_id = ?",
			responseTime, timeTaken, reference, connectionID,
		)
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to update response log: %w", err)
	}

	return err
}

// Add scheduled message to table (queued write)
func (s *StressTestDB) AddScheduledMessage(stressTestID int, createdAt time.Time, reference string, connectionID string, message string) error {
	return s.EnqueueScheduledMessage(stressTestID, createdAt, reference, connectionID, message)
}

// Add a request-response log entry (queued write)
func (s *StressTestDB) AddRequestLog(stressTestID int, requestTime time.Time, reference string, connectionID string) error {
	return s.EnqueueRequestLog(stressTestID, requestTime, reference, connectionID)
}

// Get stress test by ID
func (s *StressTestDB) GetStressTest(id int) (*domain.StressTest, error) {
	var test domain.StressTest
	err := s.db.QueryRow(
		"SELECT id, name, created_at, total_requests, test_time_secs, request_per_second FROM stress_test WHERE id = ?",
		id,
	).Scan(&test.ID, &test.Name, &test.CreatedAt, &test.TotalRequests, &test.TestTimeSecs, &test.RequestPerSecond)

	if err != nil {
		return nil, err
	}
	return &test, nil
}

// Get request logs for a stress test
func (s *StressTestDB) GetRequestLogs(stressTestID int) ([]domain.RequestLog, error) {
	rows, err := s.db.Query(
		"SELECT id, request_time, response_time, time_taken, created_at, stresstest_id, reference FROM request_response_log WHERE stresstest_id = ?",
		stressTestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.RequestLog
	for rows.Next() {
		var log domain.RequestLog
		err := rows.Scan(&log.ID, &log.RequestTime, &log.ResponseTime, &log.TimeTaken, &log.CreatedAt, &log.StressTestID, &log.Reference)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

package database

import (
	"fmt"
	"time"
)

// StressTest represents a stress test configuration
type StressTest struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	CreatedAt        time.Time `json:"created_at"`
	TotalRequests    int       `json:"total_requests"`
	TestTimeSecs     int       `json:"test_time_secs"`
	RequestPerSecond int       `json:"request_per_second"`
}

// RequestLog represents a single request-response log entry
type RequestLog struct {
	ID           int       `json:"id"`
	Reference    string    `json:"reference"`
	RequestTime  time.Time `json:"request_time"`
	ResponseTime time.Time `json:"response_time"`
	TimeTaken    int       `json:"time_taken"` // in milliseconds
	CreatedAt    time.Time `json:"created_at"`
	StressTestID int       `json:"stresstest_id"`
}

// Create a new stress test
func (s *StressTestDB) CreateStressTest(name string, testTimeSecs, requestsPerSecond int) (*StressTest, error) {
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

	return &StressTest{
		ID:               int(id),
		Name:             name,
		CreatedAt:        time.Now(),
		TestTimeSecs:     testTimeSecs,
		RequestPerSecond: requestsPerSecond,
	}, nil
}

func (s *StressTestDB) UpdateResponseTime(reference string) error {
	_, err := s.db.Exec(
		"UPDATE request_response_log set response_time = ? where reference = ?",
		time.Now(), reference,
	)
	if err != nil {
		fmt.Println(err.Error())
	}

	return err
}

// Add a request-response log entry
func (s *StressTestDB) AddRequestLog(stressTestID int, requestTime, responseTime time.Time, timeTaken int, reference string) error {
	_, err := s.db.Exec(
		"INSERT INTO request_response_log (stresstest_id, request_time, response_time, time_taken, reference) VALUES (?, ?, ?, ?, ?)",
		stressTestID, requestTime, responseTime, timeTaken, reference,
	)
	//fmt.Println(err.Error())
	return err
}

// Get stress test by ID
func (s *StressTestDB) GetStressTest(id int) (*StressTest, error) {
	var test StressTest
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
func (s *StressTestDB) GetRequestLogs(stressTestID int) ([]RequestLog, error) {
	rows, err := s.db.Query(
		"SELECT id, request_time, response_time, time_taken, created_at, stresstest_id, reference FROM request_response_log WHERE stresstest_id = ?",
		stressTestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []RequestLog
	for rows.Next() {
		var log RequestLog
		err := rows.Scan(&log.ID, &log.RequestTime, &log.ResponseTime, &log.TimeTaken, &log.CreatedAt, &log.StressTestID, &log.Reference)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

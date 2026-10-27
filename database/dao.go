package database

import (
	"database/sql"
	"fmt"
	"math"
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

// The write functions below queue to the batch writer and return once queued; write errors are logged
// by the writer. They are applied in call order, so a request's INSERT always lands before its UPDATE.

// UpdateResponseTime records when the request was written to the socket and when its response was read.
// Both times are captured by the caller so database delays are not counted as latency.
func (s *StressTestDB) UpdateResponseTime(reference string, connectionID string, requestTime, responseTime time.Time) error {
	timeTaken := responseTime.Sub(requestTime).Milliseconds()
	return s.writer.enqueue(writeOp{
		query:     "UPDATE request_response_log SET request_time = ?, response_time = ?, time_taken = ? WHERE reference = ? AND connection_id = ?",
		args:      []any{requestTime, responseTime, timeTaken, reference, connectionID},
		noRowsErr: fmt.Sprintf("no request logged for reference %s on %s", reference, connectionID),
	})
}

// Delete a request log entry for a message that was never sent
func (s *StressTestDB) DeleteRequestLog(reference string, connectionID string) error {
	return s.writer.enqueue(writeOp{
		query: "DELETE FROM request_response_log WHERE reference = ? AND connection_id = ?",
		args:  []any{reference, connectionID},
	})
}

// Add scheduled message to table
func (s *StressTestDB) AddScheduledMessage(stressTestID int, createdAt time.Time, reference string, connectionID string, message string) error {
	return s.writer.enqueue(writeOp{
		query: "INSERT INTO scheduled_message(stresstest_id, created_at, reference, connection_id, message) VALUES (?,?,?,?,?)",
		args:  []any{stressTestID, createdAt, reference, connectionID, message},
	})
}

// Add a request-response log entry
func (s *StressTestDB) AddRequestLog(stressTestID int, requestTime time.Time, reference string, connectionID string) error {
	return s.writer.enqueue(writeOp{
		query: "INSERT INTO request_response_log (stresstest_id, request_time, reference,connection_id) VALUES (?, ?, ?, ?)",
		args:  []any{stressTestID, requestTime, reference, connectionID},
	})
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

// Get request logs for a stress test. Unanswered requests have a zero ResponseTime and TimeTaken.
func (s *StressTestDB) GetRequestLogs(stressTestID int) ([]domain.RequestLog, error) {
	rows, err := s.db.Query(
		"SELECT id, request_time, response_time, time_taken, created_at, stresstest_id, reference, connection_id FROM request_response_log WHERE stresstest_id = ?",
		stressTestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []domain.RequestLog
	for rows.Next() {
		var log domain.RequestLog
		// response_time and time_taken stay NULL until a response arrives
		var responseTime sql.NullTime
		var timeTaken sql.NullInt64
		err := rows.Scan(&log.ID, &log.RequestTime, &responseTime, &timeTaken, &log.CreatedAt, &log.StressTestID, &log.Reference, &log.ConnectionID)
		if err != nil {
			return nil, err
		}
		log.ResponseTime = responseTime.Time
		log.TimeTaken = int(timeTaken.Int64)
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// SetTotalRequests records how many requests a finished stress test sent
func (s *StressTestDB) SetTotalRequests(stressTestID int, total int) error {
	return s.writer.enqueue(writeOp{
		query: "UPDATE stress_test SET total_requests = ? WHERE id = ?",
		args:  []any{total, stressTestID},
	})
}

// GetStressTestResults summarises a stress test's request log: counts, achieved rate and latency
// percentiles. It returns sql.ErrNoRows if the test does not exist.
func (s *StressTestDB) GetStressTestResults(stressTestID int) (*domain.StressTestResults, error) {
	test, err := s.GetStressTest(stressTestID)
	if err != nil {
		return nil, err
	}
	results := &domain.StressTestResults{StressTest: *test}

	var minMs, avgMs, maxMs sql.NullFloat64
	err = s.db.QueryRow(
		"SELECT count(*), count(time_taken), min(time_taken), avg(time_taken), max(time_taken) FROM request_response_log WHERE stresstest_id = ?",
		stressTestID,
	).Scan(&results.Logged, &results.Answered, &minMs, &avgMs, &maxMs)
	if err != nil {
		return nil, err
	}
	results.Unanswered = results.Logged - results.Answered
	if test.TestTimeSecs > 0 {
		// Divide by how long the test has actually run, so the rate is also right while it is still running
		secs := float64(test.TestTimeSecs)
		if running := time.Since(test.CreatedAt).Seconds(); running < secs {
			secs = math.Max(running, 1)
		}
		results.AchievedPerSecond = float64(results.Logged) / secs
	}
	results.LatencyMs.Min, results.LatencyMs.Avg, results.LatencyMs.Max = minMs.Float64, avgMs.Float64, maxMs.Float64

	// Percentiles from one sorted read rather than one sort per percentile
	rows, err := s.db.Query(
		"SELECT time_taken FROM request_response_log WHERE stresstest_id = ? AND time_taken IS NOT NULL ORDER BY time_taken",
		stressTestID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latencies := make([]int64, 0, results.Answered)
	for rows.Next() {
		var ms int64
		if err := rows.Scan(&ms); err != nil {
			return nil, err
		}
		latencies = append(latencies, ms)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	results.LatencyMs.P50 = percentile(latencies, 0.50)
	results.LatencyMs.P95 = percentile(latencies, 0.95)
	results.LatencyMs.P99 = percentile(latencies, 0.99)
	return results, nil
}

// percentile returns the nearest-rank percentile p (0-1] of sorted values, or 0 if there are none
func percentile(sorted []int64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	return float64(sorted[rank-1])
}

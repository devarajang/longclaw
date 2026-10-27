package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	values := make([]int64, 100)
	for i := range values {
		values[i] = int64(i + 1)
	}
	for _, c := range []struct {
		p    float64
		want float64
	}{{0.50, 50}, {0.95, 95}, {0.99, 99}, {1, 100}} {
		if got := percentile(values, c.p); got != c.want {
			t.Errorf("percentile(1..100, %v) = %v, want %v", c.p, got, c.want)
		}
	}
	if got := percentile(nil, 0.5); got != 0 {
		t.Errorf("percentile(empty) = %v, want 0", got)
	}
	if got := percentile([]int64{7}, 0.99); got != 7 {
		t.Errorf("percentile([7], 0.99) = %v, want 7", got)
	}
}

// newResultsTestDB creates only the tables the results queries use, so the test does not depend on
// the full schema in NewStressTestDB.
func newResultsTestDB(t *testing.T) *StressTestDB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, ddl := range []string{
		`CREATE TABLE stress_test (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			total_requests INTEGER DEFAULT 0, test_time_secs INTEGER NOT NULL, request_per_second INTEGER NOT NULL)`,
		`CREATE TABLE request_response_log (id INTEGER PRIMARY KEY AUTOINCREMENT, request_time DATETIME NOT NULL, response_time DATETIME,
			time_taken INTEGER, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, stresstest_id INTEGER NOT NULL, reference TEXT NOT NULL,
			connection_id TEXT NOT NULL)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	return &StressTestDB{db: db, writer: newBatchWriter(db)}
}

// One test with 10 answered requests (10..100ms) and 2 unanswered, plus another test that must be ignored.
func seedResults(t *testing.T, s *StressTestDB) int {
	t.Helper()
	test, err := s.CreateStressTest("results", 4, 3)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 1; i <= 10; i++ {
		s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, response_time, time_taken, reference, connection_id) VALUES (?, ?, ?, ?, ?, 'c1')",
			test.ID, now, now, i*10, i)
	}
	for i := 11; i <= 12; i++ {
		s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, reference, connection_id) VALUES (?, ?, ?, 'c1')", test.ID, now, i)
	}
	s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, time_taken, reference, connection_id) VALUES (?, ?, 9999, 'other', 'c1')", test.ID+1, now)
	return test.ID
}

func TestGetStressTestResults(t *testing.T) {
	s := newResultsTestDB(t)
	id := seedResults(t, s)

	r, err := s.GetStressTestResults(id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Logged != 12 || r.Answered != 10 || r.Unanswered != 2 {
		t.Errorf("logged/answered/unanswered = %d/%d/%d, want 12/10/2", r.Logged, r.Answered, r.Unanswered)
	}
	// Just created, so it has run under a second: the rate divides by the 1s minimum, not the full 4s
	if r.AchievedPerSecond != 12 {
		t.Errorf("achieved_per_second while running = %v, want 12 (12 requests / 1s)", r.AchievedPerSecond)
	}
	s.db.Exec("UPDATE stress_test SET created_at = datetime('now', '-1 hour') WHERE id = ?", id)
	if finished, err := s.GetStressTestResults(id); err != nil || finished.AchievedPerSecond != 3 {
		t.Errorf("achieved_per_second when finished = %v (err %v), want 3 (12 requests / 4s)", finished.AchievedPerSecond, err)
	}
	want := map[string][2]float64{"min": {r.LatencyMs.Min, 10}, "avg": {r.LatencyMs.Avg, 55}, "p50": {r.LatencyMs.P50, 50},
		"p95": {r.LatencyMs.P95, 100}, "p99": {r.LatencyMs.P99, 100}, "max": {r.LatencyMs.Max, 100}}
	for name, v := range want {
		if v[0] != v[1] {
			t.Errorf("latency %s = %v, want %v", name, v[0], v[1])
		}
	}

	if _, err := s.GetStressTestResults(id + 100); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("results for a missing test: err = %v, want sql.ErrNoRows", err)
	}
}

func TestGetRequestLogsWithUnansweredRows(t *testing.T) {
	s := newResultsTestDB(t)
	id := seedResults(t, s)

	logs, err := s.GetRequestLogs(id)
	if err != nil {
		t.Fatalf("GetRequestLogs: %v", err)
	}
	if len(logs) != 12 {
		t.Fatalf("got %d logs, want 12", len(logs))
	}
	unanswered := 0
	for _, l := range logs {
		if l.ConnectionID != "c1" {
			t.Errorf("connection_id = %q, want c1", l.ConnectionID)
		}
		if l.ResponseTime.IsZero() && l.TimeTaken == 0 {
			unanswered++
		}
	}
	if unanswered != 2 {
		t.Errorf("%d logs have no response, want 2", unanswered)
	}
}

func TestSetTotalRequests(t *testing.T) {
	s := newResultsTestDB(t)
	id := seedResults(t, s)
	if err := s.SetTotalRequests(id, 12); err != nil {
		t.Fatal(err)
	}
	s.writer.Close() // flush the queued update

	test, err := s.GetStressTest(id)
	if err != nil {
		t.Fatal(err)
	}
	if test.TotalRequests != 12 {
		t.Errorf("total_requests = %d, want 12", test.TotalRequests)
	}
}

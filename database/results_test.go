package database

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/devarajang/longclaw/internal/domain"
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
			total_requests INTEGER DEFAULT 0, test_time_secs INTEGER NOT NULL, request_per_second INTEGER NOT NULL,
			node_test_mode TEXT NOT NULL DEFAULT 'advertised')`,
		`CREATE TABLE request_response_log (id INTEGER PRIMARY KEY AUTOINCREMENT, request_time DATETIME NOT NULL, response_time DATETIME,
			time_taken INTEGER, created_at DATETIME DEFAULT CURRENT_TIMESTAMP, stresstest_id INTEGER NOT NULL, reference TEXT NOT NULL,
			connection_id TEXT NOT NULL, mti TEXT)`,
		`CREATE TABLE scheduled_message (id INTEGER PRIMARY KEY AUTOINCREMENT, reference TEXT NOT NULL, message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP, connection_id TEXT NOT NULL, stresstest_id INTEGER NOT NULL, sent_at DATETIME DEFAULT NULL)`,
	} {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err := addRefundColumns(db); err != nil {
		t.Fatal(err)
	}
	return &StressTestDB{db: db, writer: newBatchWriter(db)}
}

// One test with 10 answered requests (10..100ms) and 2 unanswered, plus another test that must be ignored.
// Originals are 0100 (8 answered at 10..80ms, 2 unanswered); follow-ups are 0420 (2 answered at 90 and 100ms).
// Of 3 scheduled follow-ups, 1 was never sent.
func seedResults(t *testing.T, s *StressTestDB) int {
	t.Helper()
	test, err := s.CreateStressTest("results", 4, 3, "advertised", "sale", 0)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i := 1; i <= 10; i++ {
		mti := "0100"
		if i > 8 {
			mti = "0420"
		}
		s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, response_time, time_taken, reference, connection_id, mti) VALUES (?, ?, ?, ?, ?, 'c1', ?)",
			test.ID, now, now, i*10, i, mti)
	}
	for i := 11; i <= 12; i++ {
		s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, reference, connection_id, mti) VALUES (?, ?, ?, 'c1', '0100')", test.ID, now, i)
	}
	for i, sentAt := range []any{now, now, nil} {
		s.db.Exec("INSERT INTO scheduled_message (stresstest_id, reference, connection_id, message, sent_at) VALUES (?, ?, 'c1', 'msg', ?)", test.ID, i+1, sentAt)
	}
	s.db.Exec("INSERT INTO scheduled_message (stresstest_id, reference, connection_id, message) VALUES (?, 'other', 'c1', 'msg')", test.ID+1)
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

	if r.FollowUpsNotSent != 1 {
		t.Errorf("follow_ups_not_sent = %d, want 1", r.FollowUpsNotSent)
	}
	originals, followUps := r.ByMTI["0100"], r.ByMTI["0420"]
	if len(r.ByMTI) != 2 || originals.Logged != 10 || originals.Answered != 8 || originals.Unanswered != 2 || followUps.Logged != 2 || followUps.Answered != 2 {
		t.Errorf("by_mti counts = %+v, want 0100 10/8/2 and 0420 2/2/0", r.ByMTI)
	}
	if got := originals.LatencyMs; got.Min != 10 || got.Avg != 45 || got.P50 != 40 || got.P95 != 80 || got.Max != 80 {
		t.Errorf("0100 latency = %+v, want min 10 avg 45 p50 40 p95 80 max 80", got)
	}
	if got := followUps.LatencyMs; got.Min != 90 || got.Avg != 95 || got.P50 != 90 || got.P99 != 100 || got.Max != 100 {
		t.Errorf("0420 latency = %+v, want min 90 avg 95 p50 90 p99 100 max 100", got)
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

// An original and its follow-up can share reference and MTI (e.g. 0200 then 0200). The follow-up's response
// must update only the follow-up's row, not overwrite the original's latency.
func TestUpdateResponseTimeOnlyUpdatesUnansweredRow(t *testing.T) {
	s := newResultsTestDB(t)
	start := time.Now()
	entry := domain.RequestLog{StressTestID: 1, RequestTime: start, Reference: "ref", ConnectionID: "c1", MTI: "0200"}
	s.AddRequestLog(entry)
	s.UpdateResponseTime("ref", "c1", "0200", start, start.Add(10*time.Millisecond), "00", "")
	s.AddRequestLog(entry) // follow-up, sent after the original was answered
	s.UpdateResponseTime("ref", "c1", "0200", start, start.Add(70*time.Millisecond), "00", "")
	s.writer.Close()

	rows, err := s.db.Query("SELECT time_taken FROM request_response_log ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []int
	for rows.Next() {
		var ms int
		rows.Scan(&ms)
		got = append(got, ms)
	}
	if len(got) != 2 || got[0] != 10 || got[1] != 70 {
		t.Errorf("time_taken per row = %v, want [10 70]", got)
	}
}

func TestAddColumnIfMissing(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A request_response_log created before the mti column existed
	db.Exec("CREATE TABLE request_response_log (id INTEGER PRIMARY KEY, reference TEXT)")
	db.Exec("INSERT INTO request_response_log (reference) VALUES ('old')")

	for i := 0; i < 2; i++ { // the second call must be a no-op
		if err := addColumnIfMissing(db, "request_response_log", "mti", "TEXT"); err != nil {
			t.Fatalf("call %d: %v", i+1, err)
		}
	}
	var mti sql.NullString
	if err := db.QueryRow("SELECT mti FROM request_response_log WHERE reference = 'old'").Scan(&mti); err != nil || mti.Valid {
		t.Errorf("existing row mti = %v (err %v), want NULL", mti, err)
	}
}

func TestNodeTestModeIsStored(t *testing.T) {
	s := newResultsTestDB(t)
	created, err := s.CreateStressTest("rr", 5, 4, "round_robin", "sale", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetStressTest(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeTestMode != "round_robin" || created.NodeTestMode != "round_robin" {
		t.Errorf("node_test_mode stored %q, returned %q, want round_robin", got.NodeTestMode, created.NodeTestMode)
	}
}

// Tests saved before node_test_mode existed ran with every node at the full rate, i.e. advertised
func TestNodeTestModeMigrationDefaultsToAdvertised(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec(`CREATE TABLE stress_test (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		total_requests INTEGER DEFAULT 0, test_time_secs INTEGER NOT NULL, request_per_second INTEGER NOT NULL)`)
	db.Exec("INSERT INTO stress_test (name, test_time_secs, request_per_second) VALUES ('old', 5, 4)")

	if err := addColumnIfMissing(db, "stress_test", "node_test_mode", "TEXT NOT NULL DEFAULT 'advertised'"); err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE request_response_log (id INTEGER PRIMARY KEY, stresstest_id INTEGER, reference TEXT, mti TEXT)")
	if err := addRefundColumns(db); err != nil {
		t.Fatal(err)
	}
	s := &StressTestDB{db: db}
	got, err := s.GetStressTest(1)
	if err != nil {
		t.Fatal(err)
	}
	if got.NodeTestMode != "advertised" {
		t.Errorf("old test node_test_mode = %q, want advertised", got.NodeTestMode)
	}
	if got.TestType != "sale" || got.SourceTestID != 0 {
		t.Errorf("old test test_type/source_test_id = %q/%d, want sale/0", got.TestType, got.SourceTestID)
	}
}

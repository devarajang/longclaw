package database

import (
	"testing"
	"time"

	"github.com/devarajang/longclaw/internal/domain"
)

// Only approved purchases that were neither reversed nor already refunded (or refund pending) are candidates
func TestGetRefundCandidates(t *testing.T) {
	s := newResultsTestDB(t)
	now := time.Now()
	logSale := func(test int, ref, mti, processingCode, responseCode string) {
		s.AddRequestLog(domain.RequestLog{StressTestID: test, RequestTime: now, Reference: ref, ConnectionID: "c1", MTI: mti,
			ProcessingCode: processingCode, RequestMessage: "msg-" + ref})
		if responseCode != "" {
			s.UpdateResponseTime(ref, "c1", mti, now, now, responseCode, "123456")
		}
	}
	logSale(1, "approved", "0200", "000000", "00")
	logSale(1, "auth", "0100", "003000", "00")
	logSale(1, "declined", "0200", "000000", "05")
	logSale(1, "unanswered", "0200", "000000", "")
	logSale(1, "not-purchase", "0200", "010000", "00")
	logSale(1, "reversed", "0200", "000000", "00")
	logSale(1, "reversed", "0420", "000000", "00")
	logSale(1, "refunded", "0200", "000000", "00")
	logSale(1, "refund-failed", "0200", "000000", "00")
	logSale(1, "refund-pending", "0200", "000000", "00")
	logSale(2, "other-test", "0200", "000000", "00")
	s.writer.Close()

	// Refunds from an earlier refund test (id 3) of the last three sales
	for _, r := range []struct {
		original     string
		responseCode any
	}{{"refunded", "00"}, {"refund-failed", "05"}, {"refund-pending", nil}} {
		var id int
		s.db.QueryRow("SELECT id FROM request_response_log WHERE reference = ?", r.original).Scan(&id)
		s.db.Exec("INSERT INTO request_response_log (stresstest_id, request_time, reference, connection_id, mti, response_code, original_log_id) VALUES (3, ?, ?, 'c1', '0200', ?, ?)",
			now, "rf-"+r.original, r.responseCode, id)
	}

	candidates, err := s.GetRefundCandidates(1)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range candidates {
		got = append(got, c.Reference)
		if c.RequestMessage != "msg-"+c.Reference {
			t.Errorf("%s: request_message = %q", c.Reference, c.RequestMessage)
		}
	}
	want := []string{"approved", "auth", "refund-failed"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("candidates = %v, want %v", got, want)
	}
}

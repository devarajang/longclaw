package network

import (
	"log"
	"time"

	"github.com/devarajang/longclaw/internal/domain"
	"github.com/devarajang/longclaw/iso"
	"github.com/devarajang/longclaw/utils"
)

// RunRefund sends one refund (DE3 20xxxx) per approved sale of refundTest.SourceTestID, at
// RequestPerSecond in total, rotating across the connected clients. It ends when every sale has
// been sent, so test_time_secs is not used.
func (server *IsoServer) RunRefund(refundTest domain.StressTest, isoSpec *iso.IsoSpec) {
	candidates, err := server.db.GetRefundCandidates(refundTest.SourceTestID)
	if err != nil {
		log.Println("Failed to load refund candidates:", err)
		return
	}
	conns := server.connections()
	if len(conns) == 0 {
		log.Println("Refund test aborted: no connected clients")
		return
	}
	log.Printf("Refund test %d: refunding %d transactions from test %d", refundTest.ID, len(candidates), refundTest.SourceTestID)

	// Same pacing as RunStress: work out how many are due from elapsed time, ticking no faster than 1ms
	interval := time.Second / time.Duration(refundTest.RequestPerSecond)
	if interval < time.Millisecond {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	sent := 0
	for sent < len(candidates) {
		now := <-ticker.C
		due := min(int(now.Sub(start).Seconds()*float64(refundTest.RequestPerSecond)), len(candidates))
		for ; sent < due; sent++ {
			sendRefund(conns[sent%len(conns)], refundTest, candidates[sent], isoSpec)
		}
	}
	log.Printf("Refund test completed: %d refunds sent in %v", sent, time.Since(start).Round(time.Millisecond))
	if err := server.db.SetTotalRequests(refundTest.ID, sent); err != nil {
		log.Println("DB Write error:", err)
	}
}

// sendRefund replays a logged sale as a refund with a new RRN, STAN and transmission time, and links
// its log row to the sale's
func sendRefund(conn *IsoConnection, refundTest domain.StressTest, original domain.RefundCandidate, isoSpec *iso.IsoSpec) {
	refund, err := iso.NewIso8583Message(original.RequestMessage, isoSpec)
	if err != nil {
		log.Println("Refund parse error:", err)
		return
	}
	reference := utils.GenerateTimestampID()
	refund.SetField(2, "20"+refund.GetField(2)[2:]) // DE3: 20xxxx = refund
	if refund.GetField(6) != "" {
		refund.SetField(6, time.Now().Format("0102150405")) // DE7
	}
	if refund.GetField(10) != "" {
		refund.SetField(10, reference[6:]) // DE11: last 6 digits of the new RRN
	}
	refund.SetField(36, reference) // DE37: new RRN

	connectionID := conn.conn.RemoteAddr().String()
	entry := newRequestLog(refundTest.ID, reference, connectionID, refund)
	entry.OriginalLogID = original.LogID
	// Log before enqueueing so the row exists before the response can arrive
	if err := conn.db.AddRequestLog(entry); err != nil {
		log.Println("DB Write error:", err)
		return
	}
	select {
	case conn.writeChannel <- IsoRequestResponse{reference: reference, mti: entry.MTI, stressTestId: refundTest.ID, rawRequest: []byte(entry.RequestMessage)}:
	default:
		droppedCount.Add(1)
		// Never sent, so a re-run of the refund test retries this sale
		if err := conn.db.DeleteRequestLog(reference, connectionID, entry.MTI); err != nil {
			log.Println("DB Write error:", err)
		}
	}
}

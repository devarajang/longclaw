package network

import (
	"log"
	"sync/atomic"
	"time"
)

// sentCount counts messages written to client sockets since the last report.
var sentCount atomic.Int64

// droppedCount counts messages not sent because a connection's write channel was full.
var droppedCount atomic.Int64

// unmatchedCount counts responses whose reference matched no request sent on that connection.
var unmatchedCount atomic.Int64

// reportSentPerMinute logs how many messages were sent, dropped and unmatched in each one-minute window.
func reportSentPerMinute() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if n := sentCount.Swap(0); n > 0 {
			log.Printf("Messages sent in last minute: %d", n)
		}
		if n := droppedCount.Swap(0); n > 0 {
			log.Printf("Messages dropped (write channel full) in last minute: %d", n)
		}
		if n := unmatchedCount.Swap(0); n > 0 {
			log.Printf("Responses with no matching request in last minute: %d", n)
		}
	}
}

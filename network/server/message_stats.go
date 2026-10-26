package network

import (
	"log"
	"sync/atomic"
	"time"
)

// sentCount counts messages written to client sockets since the last report.
var sentCount atomic.Int64

// reportSentPerMinute logs how many messages were sent in each one-minute window.
func reportSentPerMinute() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if n := sentCount.Swap(0); n > 0 {
			log.Printf("Messages sent in last minute: %d", n)
		}
	}
}

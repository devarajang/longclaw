package database

import (
	"log"
	"time"
)

type RequestLogEntry struct {
	StressTestID int
	RequestTime  time.Time
	Reference    string
	ConnectionID string
}

type ScheduledMessageEntry struct {
	StressTestID int
	CreatedAt    time.Time
	Reference    string
	ConnectionID string
	Message      string
}

// StartDBWriters initializes background write workers
func (s *StressTestDB) StartDBWriters() {
	// Create buffered channels for write requests
	requestLogQueue := make(chan RequestLogEntry, 2000)
	scheduledMsgQueue := make(chan ScheduledMessageEntry, 2000)

	// Start background writer goroutines (serialize writes to avoid SQLite lock contention)
	go s.requestLogWriter(requestLogQueue)
	go s.scheduledMessageWriter(scheduledMsgQueue)

	// Store channels for use
	s.requestLogQueue = requestLogQueue
	s.scheduledMsgQueue = scheduledMsgQueue
}

// requestLogWriter processes request logs from queue serially
func (s *StressTestDB) requestLogWriter(queue chan RequestLogEntry) {
	for entry := range queue {
		err := withSQLiteBusyRetry(func() error {
			_, execErr := s.db.Exec(
				"INSERT INTO request_response_log (stresstest_id, request_time, reference, connection_id) VALUES (?, ?, ?, ?)",
				entry.StressTestID, entry.RequestTime, entry.Reference, entry.ConnectionID)
			return execErr
		})
		if err != nil {
			log.Printf("[DB_WRITE_ERROR] Failed to log request ref=%s: %v", entry.Reference, err)
		}
	}
}

// scheduledMessageWriter processes scheduled messages from queue serially
func (s *StressTestDB) scheduledMessageWriter(queue chan ScheduledMessageEntry) {
	for entry := range queue {
		err := withSQLiteBusyRetry(func() error {
			_, execErr := s.db.Exec(
				"INSERT INTO scheduled_message(stresstest_id, created_at, reference, connection_id, message) VALUES (?,?,?,?,?)",
				entry.StressTestID, entry.CreatedAt, entry.Reference, entry.ConnectionID, entry.Message)
			return execErr
		})
		if err != nil {
			log.Printf("[DB_WRITE_ERROR] Failed to log scheduled message ref=%s: %v", entry.Reference, err)
		}
	}
}

// EnqueueRequestLog sends a request log to the write queue (non-blocking)
func (s *StressTestDB) EnqueueRequestLog(stressTestID int, requestTime time.Time, reference string, connectionID string) error {
	select {
	case s.requestLogQueue <- RequestLogEntry{stressTestID, requestTime, reference, connectionID}:
		return nil
	default:
		log.Printf("[QUEUE_DROP] Request log queue full, dropping ref=%s", reference)
		return nil // Silently drop instead of blocking
	}
}

// EnqueueScheduledMessage sends a scheduled message to the write queue (non-blocking)
func (s *StressTestDB) EnqueueScheduledMessage(stressTestID int, createdAt time.Time, reference string, connectionID string, message string) error {
	select {
	case s.scheduledMsgQueue <- ScheduledMessageEntry{stressTestID, createdAt, reference, connectionID, message}:
		return nil
	default:
		log.Printf("[QUEUE_DROP] Scheduled message queue full, dropping ref=%s", reference)
		return nil
	}
}

package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type StressTestDB struct {
	db                *sql.DB
	requestLogQueue   chan RequestLogEntry
	scheduledMsgQueue chan ScheduledMessageEntry
}

// Initialize database and create tables if they don't exist
func NewStressTestDB(dbPath string) (*StressTestDB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database (creates if not exists)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite handles concurrency best with a single shared connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	defer func() {
		if err != nil {
			_ = db.Close()
		}
	}()

	// Enable WAL Mode
	var journalMode string
	if err = db.QueryRow(`PRAGMA journal_mode = WAL;`).Scan(&journalMode); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if journalMode != "wal" {
		return nil, fmt.Errorf("enable WAL: sqlite returned %q", journalMode)
	}

	// Improve performance for WAL mode (optional)
	if _, err = db.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return nil, fmt.Errorf("set synchronous mode: %w", err)
	}
	if _, err = db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err = db.Exec(`PRAGMA busy_timeout = 15000;`); err != nil {
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	// Create stress_test table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS stress_test (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			total_requests INTEGER DEFAULT 0,
			test_time_secs INTEGER NOT NULL,
			request_per_second INTEGER NOT NULL
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create request_response_log table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS request_response_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			request_time DATETIME NOT NULL,
			response_time DATETIME,
			time_taken INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			stresstest_id INTEGER NOT NULL,
			reference TEXT NOT NULL,
			connection_id TEXT NOT NULL,
			FOREIGN KEY (stresstest_id) REFERENCES stress_test (id)
		)
	`)
	if err != nil {
		return nil, err
	}

	// Create scheduled_message table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS scheduled_message (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			reference TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			connection_id TEXT NOT NULL,
			stresstest_id INTEGER NOT NULL,
			created_at DATETIME DEFAULT NULL,
			FOREIGN KEY (stresstest_id) REFERENCES stress_test (id)
		)
	`)
	if err != nil {
		return nil, err
	}

	sdb := &StressTestDB{
		db:                db,
		requestLogQueue:   make(chan RequestLogEntry, 2000),
		scheduledMsgQueue: make(chan ScheduledMessageEntry, 2000),
	}

	// Start background writer goroutines
	go sdb.requestLogWriter(sdb.requestLogQueue)
	go sdb.scheduledMessageWriter(sdb.scheduledMsgQueue)

	return sdb, nil
}

// Close database connection and writer goroutines
func (s *StressTestDB) Close() error {
	// Close channels to signal writers to stop
	close(s.requestLogQueue)
	close(s.scheduledMsgQueue)
	// Give writers time to flush pending items
	time.Sleep(500 * time.Millisecond)
	return s.db.Close()
}

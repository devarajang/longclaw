package database

import (
	"database/sql"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type StressTestDB struct {
	db *sql.DB
}

// Initialize database and create tables if they don't exist
func NewStressTestDB(dbPath string) (*StressTestDB, error) {
	// Open database (creates if not exists)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Enable WAL Mode
	if _, err := db.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		log.Fatal("Failed to enable WAL:", err)
	}

	// Improve performance for WAL mode (optional)
	db.Exec(`PRAGMA synchronous = NORMAL;`)
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

	return &StressTestDB{db: db}, nil
}

// Close database connection
func (s *StressTestDB) Close() error {
	return s.db.Close()
}

package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3"
)

type StressTestDB struct {
	db     *sql.DB
	writer *batchWriter // all hot-path writes go through here, batched into transactions
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
			request_per_second INTEGER NOT NULL,
			node_test_mode TEXT NOT NULL DEFAULT 'advertised'
		)
	`)
	if err != nil {
		return nil, err
	}
	// Databases created before node_test_mode existed; their tests ran in advertised mode
	if err := addColumnIfMissing(db, "stress_test", "node_test_mode", "TEXT NOT NULL DEFAULT 'advertised'"); err != nil {
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
			mti TEXT,
			FOREIGN KEY (stresstest_id) REFERENCES stress_test (id)
		)
	`)
	if err != nil {
		return nil, err
	}
	// Databases created before the mti column existed
	if err := addColumnIfMissing(db, "request_response_log", "mti", "TEXT"); err != nil {
		return nil, err
	}

	if err := addRefundColumns(db); err != nil {
		return nil, err
	}

	// UpdateResponseTime looks rows up by reference and connection; without this index every response scans the table
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_request_response_log_reference ON request_response_log (reference, connection_id)`)
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

	// MarkScheduledMessageSent looks follow-ups up by reference and connection; without this index each
	// update scans the whole table, which stalled the batch writer under load
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_scheduled_message_reference ON scheduled_message (reference, connection_id)`)
	if err != nil {
		return nil, err
	}

	return &StressTestDB{db: db, writer: newBatchWriter(db)}, nil
}

// refundColumns are the columns refund tests need: the test type, and enough of each request and
// response to find approved sales and replay them as refunds
var refundColumns = []struct{ table, column, columnType string }{
	{"stress_test", "test_type", "TEXT NOT NULL DEFAULT 'sale'"},
	{"stress_test", "source_test_id", "INTEGER"},
	{"request_response_log", "processing_code", "TEXT"},
	{"request_response_log", "pan", "TEXT"},
	{"request_response_log", "amount", "TEXT"},
	{"request_response_log", "currency", "TEXT"},
	{"request_response_log", "request_message", "TEXT"},
	{"request_response_log", "response_code", "TEXT"},
	{"request_response_log", "auth_id", "TEXT"},
	{"request_response_log", "original_log_id", "INTEGER"},
}

// addRefundColumns adds refundColumns to new and existing databases, plus the indexes GetRefundCandidates uses
func addRefundColumns(db *sql.DB) error {
	for _, c := range refundColumns {
		if err := addColumnIfMissing(db, c.table, c.column, c.columnType); err != nil {
			return fmt.Errorf("add column %s.%s: %v", c.table, c.column, err)
		}
	}
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_request_response_log_stresstest ON request_response_log (stresstest_id);
		CREATE INDEX IF NOT EXISTS idx_request_response_log_original ON request_response_log (original_log_id);
	`)
	return err
}

// addColumnIfMissing adds a column to an existing table; CREATE TABLE IF NOT EXISTS does not change
// tables created by older versions
func addColumnIfMissing(db *sql.DB, table, column, columnType string) error {
	var exists int
	if err := db.QueryRow("SELECT count(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType))
	return err
}

// Close flushes queued writes, then closes the database connection
func (s *StressTestDB) Close() error {
	s.writer.Close()
	return s.db.Close()
}

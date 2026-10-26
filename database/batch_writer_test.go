package database

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func newTestWriter(t *testing.T) (*sql.DB, *batchWriter) {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE log (ref TEXT PRIMARY KEY, answered INTEGER DEFAULT 0)`); err != nil {
		t.Fatal(err)
	}
	return db, newBatchWriter(db)
}

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// Writes for one reference must apply in call order: INSERT, then UPDATE or DELETE.
func TestBatchWriterKeepsOrder(t *testing.T) {
	db, w := newTestWriter(t)
	for i := 0; i < 5000; i++ {
		ref := fmt.Sprint(i)
		w.enqueue(writeOp{query: "INSERT INTO log (ref) VALUES (?)", args: []any{ref}})
		if i%10 == 0 {
			w.enqueue(writeOp{query: "DELETE FROM log WHERE ref = ?", args: []any{ref}})
		} else {
			w.enqueue(writeOp{query: "UPDATE log SET answered = 1 WHERE ref = ?", args: []any{ref}})
		}
	}
	w.Close()

	if got := count(t, db, "SELECT count(*) FROM log"); got != 4500 {
		t.Errorf("rows = %d, want 4500", got)
	}
	if got := count(t, db, "SELECT count(*) FROM log WHERE answered = 0"); got != 0 {
		t.Errorf("%d rows were updated before they were inserted", got)
	}
}

// Close must commit everything queued by concurrent writers.
func TestBatchWriterConcurrentFlushOnClose(t *testing.T) {
	db, w := newTestWriter(t)
	var wg sync.WaitGroup
	for g := 0; g < 50; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				ref := fmt.Sprintf("%d-%d", g, i)
				w.enqueue(writeOp{query: "INSERT INTO log (ref) VALUES (?)", args: []any{ref}})
				w.enqueue(writeOp{query: "UPDATE log SET answered = 1 WHERE ref = ?", args: []any{ref}})
			}
		}(g)
	}
	wg.Wait()
	w.Close()

	if got := count(t, db, "SELECT count(*) FROM log WHERE answered = 1"); got != 10000 {
		t.Errorf("answered rows = %d, want 10000", got)
	}
}

func TestBatchWriterRejectsAfterClose(t *testing.T) {
	_, w := newTestWriter(t)
	w.Close()
	if err := w.enqueue(writeOp{query: "INSERT INTO log (ref) VALUES ('x')"}); err != errDBClosed {
		t.Errorf("enqueue after Close = %v, want errDBClosed", err)
	}
}

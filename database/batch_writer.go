package database

import (
	"database/sql"
	"errors"
	"log"
)

// writeQueueSize is how many writes can wait for the batch writer before callers block
const writeQueueSize = 100000

// maxBatchSize caps how many writes go into one transaction
const maxBatchSize = 2000

var errDBClosed = errors.New("database is closed")

// writeOp is one queued INSERT/UPDATE/DELETE. If noRowsErr is set, it is logged when the statement changes no rows.
type writeOp struct {
	query     string
	args      []any
	noRowsErr string
}

// batchWriter runs all hot-path writes from one goroutine, grouping whatever is queued into a single
// transaction. One commit per batch instead of per statement is what lets SQLite keep up under load,
// and the single FIFO queue keeps a request's INSERT ahead of its response's UPDATE.
type batchWriter struct {
	db       *sql.DB
	writes   chan writeOp
	done     chan struct{} // closed by Close to stop accepting writes
	finished chan struct{} // closed by run after the last batch is committed
}

func newBatchWriter(db *sql.DB) *batchWriter {
	w := &batchWriter{
		db:       db,
		writes:   make(chan writeOp, writeQueueSize),
		done:     make(chan struct{}),
		finished: make(chan struct{}),
	}
	go w.run()
	return w
}

// enqueue queues a write; it blocks while the queue is full, which parks the caller in Go rather than in SQLite
func (w *batchWriter) enqueue(op writeOp) error {
	select {
	case <-w.done:
		return errDBClosed
	default:
	}
	select {
	case w.writes <- op:
		return nil
	case <-w.done:
		return errDBClosed
	}
}

func (w *batchWriter) run() {
	defer close(w.finished)
	batch := make([]writeOp, 0, maxBatchSize)
	for {
		select {
		case op := <-w.writes:
			batch = append(batch[:0], op)
		case <-w.done:
			// Flush whatever is still queued, then stop
			for {
				batch = batch[:0]
				for len(batch) < maxBatchSize && len(w.writes) > 0 {
					batch = append(batch, <-w.writes)
				}
				if len(batch) == 0 {
					return
				}
				w.commit(batch)
			}
		}
		for len(batch) < maxBatchSize && len(w.writes) > 0 {
			batch = append(batch, <-w.writes)
		}
		w.commit(batch)
	}
}

func (w *batchWriter) commit(batch []writeOp) {
	tx, err := w.db.Begin()
	if err != nil {
		log.Println("DB batch begin error:", err)
		return
	}
	stmts := make(map[string]*sql.Stmt)
	for _, op := range batch {
		stmt, ok := stmts[op.query]
		if !ok {
			if stmt, err = tx.Prepare(op.query); err != nil {
				log.Println("DB batch prepare error:", err)
				continue
			}
			stmts[op.query] = stmt
		}
		result, err := stmt.Exec(op.args...)
		if err != nil {
			log.Println("DB Write error:", err)
			continue
		}
		if op.noRowsErr != "" {
			if n, err := result.RowsAffected(); err == nil && n == 0 {
				log.Println("DB Write error:", op.noRowsErr)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		log.Println("DB batch commit error:", err)
	}
}

// Close stops accepting writes and waits until everything already queued is committed
func (w *batchWriter) Close() {
	close(w.done)
	<-w.finished
}

package database

import (
	"strings"
	"time"
)

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "database is locked")
}

func withSQLiteBusyRetry(op func() error) error {
	backoff := 10 * time.Millisecond
	for attempt := 0; attempt < 5; attempt++ {
		err := op()
		if !isSQLiteBusyError(err) {
			return err
		}
		if attempt == 4 {
			return err
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return nil
}

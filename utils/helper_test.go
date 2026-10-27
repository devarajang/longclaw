package utils

import (
	"sync"
	"testing"
)

// References go into fixed-length field 37 and must never repeat, even when generated concurrently.
func TestGenerateTimestampIDUniqueUnderConcurrency(t *testing.T) {
	const goroutines, perGoroutine = 50, 2000
	ids := make(chan string, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ids <- GenerateTimestampID()
			}
		}()
	}
	wg.Wait()
	close(ids)

	seen := make(map[string]bool, goroutines*perGoroutine)
	for id := range ids {
		if len(id) != 12 {
			t.Fatalf("reference %q has length %d, want 12", id, len(id))
		}
		for _, c := range id {
			if c < '0' || c > '9' {
				t.Fatalf("reference %q is not numeric", id)
			}
		}
		if seen[id] {
			t.Fatalf("duplicate reference %q", id)
		}
		seen[id] = true
	}
}

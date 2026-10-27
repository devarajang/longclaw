package runner

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestReserveAllowsOneTestAtATime(t *testing.T) {
	var str StressTestRunner
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if str.Reserve() {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("%d concurrent reservations succeeded, want 1", won.Load())
	}

	str.Release()
	if !str.Reserve() {
		t.Fatal("Reserve failed after Release")
	}
}

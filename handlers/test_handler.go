package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devarajang/longclaw/internal/domain"
)

func (h *Handlers) CreateTest(w http.ResponseWriter, r *http.Request) {
	// Create a new stress test (from handler)
	var stressTestReq = domain.StressTest{}
	if err := json.NewDecoder(r.Body).Decode(&stressTestReq); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
		return
	}
	// RunStress divides by request_per_second, so zero or negative values would crash the server
	if stressTestReq.TestTimeSecs <= 0 || stressTestReq.RequestPerSecond <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "test_time_secs and request_per_second must be greater than 0")
		return
	}
	stressTest, err := h.App.DB.CreateStressTest(stressTestReq.Name, stressTestReq.TestTimeSecs, stressTestReq.RequestPerSecond)
	if err != nil {
		log.Println("Failed to create stress test:", err)
		writeError(w, http.StatusInternalServerError, "db_error", "failed to create stress test")
		return
	}

	// The runner only receives while idle, so a send that cannot complete means a test is already running.
	// Waiting here would hold the HTTP request open for the rest of that test.
	select {
	case h.App.StressRunner.StressChannel <- *stressTest:
	default:
		writeError(w, http.StatusConflict, "test_in_progress", "a stress test is already running")
		return
	}
	log.Printf("Created stress test: %+v\n", stressTest)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stressTest)
}

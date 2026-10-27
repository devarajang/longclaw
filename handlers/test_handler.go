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
	// Reserve before inserting so a request rejected because a test is running leaves no stress_test row
	if !h.App.StressRunner.Reserve() {
		writeError(w, http.StatusConflict, "test_in_progress", "a stress test is already running")
		return
	}
	stressTest, err := h.App.DB.CreateStressTest(stressTestReq.Name, stressTestReq.TestTimeSecs, stressTestReq.RequestPerSecond)
	if err != nil {
		h.App.StressRunner.Release()
		log.Println("Failed to create stress test:", err)
		writeError(w, http.StatusInternalServerError, "db_error", "failed to create stress test")
		return
	}

	// The reservation guarantees the runner is idle, so this only waits for it to loop back to receive
	h.App.StressRunner.StressChannel <- *stressTest
	log.Printf("Created stress test: %+v\n", stressTest)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stressTest)
}

package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

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
	switch stressTestReq.NodeTestMode {
	case "":
		stressTestReq.NodeTestMode = domain.NodeTestModeAdvertised
	case domain.NodeTestModeAdvertised, domain.NodeTestModeRoundRobin:
	default:
		writeError(w, http.StatusBadRequest, "invalid_request", "node_test_mode must be advertised or round_robin")
		return
	}
	// Reserve before inserting so a request rejected because a test is running leaves no stress_test row
	if !h.App.StressRunner.Reserve() {
		writeError(w, http.StatusConflict, "test_in_progress", "a stress test is already running")
		return
	}
	stressTest, err := h.App.DB.CreateStressTest(stressTestReq.Name, stressTestReq.TestTimeSecs, stressTestReq.RequestPerSecond, stressTestReq.NodeTestMode)
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

// GetTestResults returns the summary for one stress test; it can be polled while the test runs
func (h *Handlers) GetTestResults(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "stress test id must be a number")
		return
	}
	results, err := h.App.DB.GetStressTestResults(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "no stress test with id "+strconv.Itoa(id))
		return
	}
	if err != nil {
		log.Println("Failed to read stress test results:", err)
		writeError(w, http.StatusInternalServerError, "db_error", "failed to read stress test results")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

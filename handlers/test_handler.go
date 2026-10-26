package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devarajang/longclaw/dtos"
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
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(dtos.ErrorResponse{
			Error:   "invalid_request",
			Message: "test_time_secs and request_per_second must be greater than 0",
		})
		return
	}
	stressTest, err := h.App.DB.CreateStressTest(stressTestReq.Name, stressTestReq.TestTimeSecs, stressTestReq.RequestPerSecond)
	if err != nil {
		log.Fatal("Failed to create stress test:", err)
	}

	h.App.StressRunner.StressChannel <- *stressTest
	log.Printf("Created stress test: %+v\n", stressTest)
	json.NewEncoder(w).Encode(stressTest)
}

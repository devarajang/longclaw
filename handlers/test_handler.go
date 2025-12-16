package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/devarajang/longclaw/database"
)

func (h *Handlers) CreateTest(w http.ResponseWriter, r *http.Request) {
	// Create a new stress test (from handler)
	var stressTestReq = database.StressTest{}
	if err := json.NewDecoder(r.Body).Decode(&stressTestReq); err != nil {
		http.Error(w, `{"error": "Invalid JSON"}`, http.StatusBadRequest)
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

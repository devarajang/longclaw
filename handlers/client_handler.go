package handlers

import (
	"encoding/json"
	"net/http"
)

func (h *Handlers) GetClients(w http.ResponseWriter, r *http.Request) {
	clients := h.App.IsoServer.GetConnectedClients()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (h *Handlers) TestClient(w http.ResponseWriter, r *http.Request) {
	strList := make([]string, 0)

	// Body is a JSON array of client IDs; only the first is tested
	if err := json.NewDecoder(r.Body).Decode(&strList); err != nil || len(strList) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", `body must be a JSON array of client IDs, e.g. ["127.0.0.1:50000"]`)
		return
	}

	_, err := h.App.IsoServer.TestConnection(strList[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.App.IsoServer.GetConnectedClients())
}

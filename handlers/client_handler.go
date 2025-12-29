package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/devarajang/longclaw/dtos"
)

func (h *Handlers) GetClients(w http.ResponseWriter, r *http.Request) {
	clients := h.App.IsoServer.GetConnectedClients()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

func (h *Handlers) TestClient(w http.ResponseWriter, r *http.Request) {
	strList := make([]string, 0)

	json.NewDecoder(r.Body).Decode(&strList)

	_, err := h.App.IsoServer.TestConnection(strList[0])
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		resp := dtos.ErrorResponse{
			Error:   "invalid_request",
			Message: err.Error(),
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(h.App.IsoServer.GetConnectedClients())
}

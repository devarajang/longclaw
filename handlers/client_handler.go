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

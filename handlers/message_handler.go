package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// messageTemplates holds the ISO 8583 network management messages.
// STAN is a 6-digit number derived from HHmmss.
var messageTemplates = map[string]func() string{
	"signon": func() string {
		// MMDDHHmmss
		trandate := time.Now().Format("0102150405")
		stan := time.Now().Format("150405")
		return "080082200000000000000400000100000000" + trandate + stan + "07100022020"
	},
	"signoff": func() string {
		trandate := time.Now().Format("0102150405")
		stan := time.Now().Format("150405")
		return "080082200000000000000400000100000000" + trandate + stan + "07200022020"
	},
	"echo": func() string {
		trandate := time.Now().Format("0102150405")
		stan := time.Now().Format("150405")
		return "080082200000000000000400000100000000" + trandate + stan + "27000022020"
	},
}

type SendMessageRequest struct {
	ClientID    string `json:"client_id"`
	MessageType string `json:"message_type"` // signon, signoff, echo
}

func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	msgFn, ok := messageTemplates[req.MessageType]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "unknown message_type: " + req.MessageType})
		return
	}

	if err := h.App.IsoServer.SendMessage(req.ClientID, msgFn()); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "sent",
		"client_id":    req.ClientID,
		"message_type": req.MessageType,
	})
}

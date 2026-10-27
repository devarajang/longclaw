package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/dtos"
	network "github.com/devarajang/longclaw/network/server"
	"github.com/devarajang/longclaw/runner"
)

type AppConfig struct {
	BasePath string
	DataPath string
	CertPath string
}

type App struct {
	Config       *AppConfig
	DB           *database.StressTestDB
	IsoServer    *network.IsoServer
	StressRunner *runner.StressTestRunner
}

type Handlers struct {
	Version string
	App     *App
	Mux     *http.ServeMux
}

func New(version string, app *App) *Handlers {
	return &Handlers{App: app,
		Mux:     http.NewServeMux(),
		Version: version}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(h.Version))
}

// writeError sends a JSON dtos.ErrorResponse with the given HTTP status
func writeError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(dtos.ErrorResponse{Error: code, Message: message})
}

// StartServer serves HTTP until ctx is cancelled, then shuts down gracefully, letting in-flight requests
// finish for up to 5s. It returns early with the error if the server cannot start (e.g. port in use).
func (h *Handlers) StartServer(ctx context.Context, addr string) error {
	h.routes()
	srv := &http.Server{
		Addr:              addr,
		Handler:           h.Mux,
		ReadHeaderTimeout: 10 * time.Second, // don't let idle clients hold connections open forever
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Println("Starting HTTP server", addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (h *Handlers) routes() {
	h.Mux.HandleFunc("/health", h.Health)
	h.Mux.HandleFunc("GET /api/clients", h.GetClients)

	h.Mux.HandleFunc("POST /api/clients/test", h.TestClient)

	h.Mux.HandleFunc("POST /api/stress_tests", h.CreateTest)

	h.Mux.HandleFunc("GET /api/stress_tests/{id}/results", h.GetTestResults)

	h.Mux.HandleFunc("POST /api/clients/message", h.SendMessage)

	h.Mux.HandleFunc("GET /api/system_info", h.SystemInfo)

}

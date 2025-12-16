package handlers

import (
	"net/http"

	"github.com/devarajang/longclaw/database"
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

func (h *Handlers) StartServer(addr string) {
	h.routes()
	http.ListenAndServe(addr, h.Mux)
}

func (h *Handlers) routes() {
	h.Mux.HandleFunc("/health", h.Health)
	h.Mux.HandleFunc("GET /api/clients", h.GetClients)

	h.Mux.HandleFunc("POST /api/stress_tests", h.CreateTest)

	h.Mux.HandleFunc("GET /api/system_info", h.SystemInfo)

}

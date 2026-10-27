package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/devarajang/longclaw/database"
	"github.com/devarajang/longclaw/handlers"
	network "github.com/devarajang/longclaw/network/server"
)

// serveUntilSignal serves HTTP until Ctrl-C or SIGTERM, then closes the ISO listener and connections and
// closes the database, which commits every write still queued in the batch writer. If the HTTP server
// fails instead (e.g. port in use), it runs the same cleanup and exits with status 1.
func serveUntilSignal(server *handlers.Handlers, addr string, isoServer *network.IsoServer, db *database.StressTestDB) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpErr := server.StartServer(ctx, addr)
	if httpErr != nil {
		log.Println("HTTP server stopped:", httpErr)
	}

	log.Println("Shutting down: closing ISO connections and flushing database writes")
	isoServer.Stop()
	if err := db.Close(); err != nil {
		log.Println("Failed to close database:", err)
	}
	if httpErr != nil {
		os.Exit(1)
	}
	log.Println("Shutdown complete")
}

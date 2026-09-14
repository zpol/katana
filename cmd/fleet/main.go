package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zpol/katana/internal/fleet"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("FLEET_ADDR", "0.0.0.0:8090")
	prom := env("PROMETHEUS_URL", "http://prometheus:9090")
	db := env("FLEET_DB_PATH", "/data/fleet.db")
	token := env("FLEET_TOKEN", "")
	web := env("FLEET_WEB_DIST", "web-fleet/dist")

	srv, err := fleet.New(prom, db, token, web)
	if err != nil {
		os.Stderr.WriteString("fleet: " + err.Error() + "\n")
		os.Exit(1)
	}
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv.StartSnapshotLoop(ctx)

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			os.Stderr.WriteString("listen: " + err.Error() + "\n")
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdown, done := context.WithTimeout(context.Background(), 8*time.Second)
	defer done()
	_ = httpSrv.Shutdown(shutdown)
}

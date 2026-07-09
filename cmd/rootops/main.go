package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/soul4bit/rootops/internal/config"
	"github.com/soul4bit/rootops/internal/storage"
	"github.com/soul4bit/rootops/internal/web"
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	store, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("open storage: %v", err)
	}
	defer store.Close()

	app, err := web.NewServer(cfg, store)
	if err != nil {
		log.Fatalf("create web server: %v", err)
	}

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("RootOPS server: http://%s", cfg.Addr)
		log.Printf("SQLite database: %s", cfg.DBPath)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
}

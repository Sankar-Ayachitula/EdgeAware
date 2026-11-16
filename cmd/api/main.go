package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"edgeaware/internal/cleanup"
	"edgeaware/internal/config"
	"edgeaware/internal/db"
	"edgeaware/internal/server"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	if cfg.CleanupInterval > 0 {
		go cleanup.Every(context.Background(), gdb, cfg.CleanupInterval, cleanup.Options{
			FeedRetention: cfg.FeedRetention,
			OpsRetention:  cfg.OpsRetention,
		})
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(gdb, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("server running on :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}

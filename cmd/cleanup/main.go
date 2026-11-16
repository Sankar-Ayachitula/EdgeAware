// Command cleanup runs the sync cleanup once and exits. Use it from cron (with
// CLEANUP_INTERVAL=0 on the API) if you prefer not to run it inside the API process.
//
//	go run ./cmd/cleanup
package main

import (
	"context"
	"log"

	"edgeaware/internal/cleanup"
	"edgeaware/internal/config"
	"edgeaware/internal/db"
)

func main() {
	cfg := config.Load()
	gdb, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	res, err := cleanup.Run(context.Background(), gdb, cleanup.Options{
		FeedRetention: cfg.FeedRetention,
		OpsRetention:  cfg.OpsRetention,
	})
	if err != nil {
		log.Fatalf("cleanup: %v", err)
	}
	if res.Skipped {
		log.Println("skipped: another cleanup is already running")
		return
	}
	log.Printf("deleted %d feed rows, %d saved push results", res.FeedRowsDeleted, res.OpsDeleted)
}

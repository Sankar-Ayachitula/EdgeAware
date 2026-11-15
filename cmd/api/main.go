package main

import (
	"log"
	"net/http"
	"time"

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

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(gdb, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("server running on :%s", cfg.Port)
	log.Fatal(srv.ListenAndServe())
}

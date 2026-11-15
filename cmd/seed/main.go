// Command seed creates the schema and a sample organisation in an empty database.
//
//	go run ./cmd/seed
package main

import (
	"log"

	"edgeaware/internal/config"
	"edgeaware/internal/db"
	"edgeaware/internal/seed"
)

func main() {
	gdb, err := db.Open(config.DatabaseURL())
	if err != nil {
		log.Fatal(err)
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	users, err := seed.Run(gdb)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Printf("seeded %d users (password for all: see seed.DevPassword in internal/seed/seed.go)", len(users))
}

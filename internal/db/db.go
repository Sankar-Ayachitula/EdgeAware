package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open() *sql.DB {
	// Default values match your Docker run command
	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	user := getenv("DB_USER", "postgres")
	pass := getenv("DB_PASSWORD", "postgres")
	name := getenv("DB_NAME", "edgeaware")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, name,
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open failed: %v", err)
	}

	// Sensible pool defaults for now
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping failed: %v", err)
	}

	log.Println("Connected to Postgres")
	return db
}

func getenv(key, def string) string {
	val := os.Getenv(key)
	if val == "" {
		return def
	}
	return val
}

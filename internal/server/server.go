// Package server builds the HTTP router with all modules wired together.
package server

import (
	"net/http"

	"gorm.io/gorm"

	"edgeaware/internal/auth"
	"edgeaware/internal/config"
	"edgeaware/internal/hierarchy"
	"edgeaware/internal/policy"
	"edgeaware/internal/sync"
	"edgeaware/internal/task"
	"edgeaware/internal/user"
	"edgeaware/pkg/middleware"
)

func New(db *gorm.DB, cfg config.Config) http.Handler {
	tokens := auth.NewTokens(cfg.JWTSecret, cfg.TokenTTL)
	hie := hierarchy.New(db)
	pol := policy.New(db, hie)
	protect := middleware.Auth(tokens)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /login", auth.NewHandler(db, tokens).Login)
	user.NewHandler(user.NewService(db, hie, pol)).Register(mux, protect)
	tasks := task.NewService(db, pol)
	task.NewHandler(tasks).Register(mux, protect)
	sync.NewHandler(sync.NewService(db, pol, tasks)).Register(mux, protect)
	return mux
}

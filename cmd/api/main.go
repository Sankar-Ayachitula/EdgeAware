package main

import (
	"fmt"
	"net/http"

	"edgeaware/internal/auth"
	"edgeaware/internal/db"
	"edgeaware/pkg/middleware"
)

func main() {
	sqlDB := db.Open()

	userRepo := auth.NewUserRepo(sqlDB)
	authHandler := auth.NewHandler(userRepo)

	http.HandleFunc("/login", authHandler.LoginHandler)

	protected := middleware.AuthMiddleware(http.HandlerFunc(protectedHandler))
	http.Handle("/protected", protected)

	// Optional health
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	fmt.Println("Server running on :8080")
	_ = http.ListenAndServe(":8080", nil)
}

func protectedHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("You have accessed a protected endpoint"))
}

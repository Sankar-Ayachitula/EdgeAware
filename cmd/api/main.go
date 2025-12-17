package main

import (
	"edgeaware/internal/health"
	"fmt"
	"net/http"
)

func main() {

	http.HandleFunc("/health", health.HealthHandler)

	fmt.Println("Starting server on :8080")

	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("Error starting server:", err)
	}

}

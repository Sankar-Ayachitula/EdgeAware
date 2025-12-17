package health

import (
	"fmt"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Health check endpoint hit")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// Package httpx has small JSON request/response helpers shared by handlers.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"

	"edgeaware/internal/apperr"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, map[string]string{"error": msg})
}

// Decode reads a JSON body (max 1 MB) and rejects unknown fields.
func Decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// FromError maps service errors to HTTP status codes.
func FromError(w http.ResponseWriter, err error) {
	var ve apperr.Validation
	switch {
	case errors.As(err, &ve):
		Error(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, apperr.ErrNotFound):
		Error(w, http.StatusNotFound, "not found")
	case errors.Is(err, apperr.ErrForbidden):
		Error(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, apperr.ErrConflict):
		Error(w, http.StatusConflict, err.Error())
	default:
		Error(w, http.StatusInternalServerError, "server error")
	}
}

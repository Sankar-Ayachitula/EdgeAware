package auth

import (
	"encoding/json"
	"net/http"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type Handler struct {
	users *UserRepo
}

func NewHandler(users *UserRepo) *Handler {
	return &Handler{users: users}
}

func (h *Handler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, ok, err := h.users.FindByUsername(req.Username)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if !ok || user.Password != req.Password {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := GenerateToken(user)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	resp := LoginResponse{AccessToken: token}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

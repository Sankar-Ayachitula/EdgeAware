package auth

import (
	"errors"
	"net/http"

	"gorm.io/gorm"

	"edgeaware/internal/httpx"
	"edgeaware/internal/models"
)

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AccessToken string `json:"access_token"`
}

type Handler struct {
	db     *gorm.DB
	tokens *Tokens
}

func NewHandler(db *gorm.DB, tokens *Tokens) *Handler {
	return &Handler{db: db, tokens: tokens}
}

// Login checks a username and password and returns a signed access token.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var user models.User
	err := h.db.WithContext(r.Context()).Where("username = ?", req.Username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !CheckPassword(user.PasswordHash, req.Password)) {
		httpx.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "server error")
		return
	}

	token, err := h.tokens.Generate(user.ID, user.Username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	httpx.JSON(w, http.StatusOK, LoginResponse{AccessToken: token})
}

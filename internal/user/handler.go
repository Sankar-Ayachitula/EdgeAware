package user

import (
	"net/http"

	"edgeaware/internal/httpx"
	"edgeaware/pkg/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Register mounts the routes. Every route expects the auth middleware in front.
func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /roles", protect(http.HandlerFunc(h.roles)))
	mux.Handle("GET /me", protect(http.HandlerFunc(h.me)))
	mux.Handle("POST /users", protect(http.HandlerFunc(h.create)))
	mux.Handle("GET /users/{id}", protect(http.HandlerFunc(h.get)))
	mux.Handle("GET /users/{id}/juniors", protect(http.HandlerFunc(h.juniors)))
	mux.Handle("GET /users/{id}/team", protect(http.HandlerFunc(h.team)))
	mux.Handle("PATCH /users/{id}/manager", protect(http.HandlerFunc(h.changeManager)))
}

func (h *Handler) roles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.svc.Roles(r.Context())
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, roles)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	me := middleware.UserID(r.Context())
	u, err := h.svc.Get(r.Context(), me, me)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, err := h.svc.Get(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) juniors(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.DirectJuniors(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, users)
}

func (h *Handler) team(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.Team(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, users)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	u, err := h.svc.Create(r.Context(), middleware.UserID(r.Context()), in)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, u)
}

func (h *Handler) changeManager(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ManagerID string `json:"manager_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.ChangeManager(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"), in.ManagerID); err != nil {
		httpx.FromError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

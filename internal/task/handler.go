package task

import (
	"errors"
	"net/http"
	"strconv"

	"edgeaware/internal/httpx"
	"edgeaware/pkg/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /users/{id}/tasks", protect(http.HandlerFunc(h.listForUser)))
	mux.Handle("POST /tasks", protect(http.HandlerFunc(h.create)))
	mux.Handle("GET /tasks/{id}", protect(http.HandlerFunc(h.get)))
	mux.Handle("PATCH /tasks/{id}", protect(http.HandlerFunc(h.update)))
	mux.Handle("DELETE /tasks/{id}", protect(http.HandlerFunc(h.delete)))
}

func (h *Handler) listForUser(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.svc.ListForUser(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"), r.URL.Query().Get("status"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, tasks)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in CreateInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, err := h.svc.Create(r.Context(), middleware.UserID(r.Context()), in)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, t)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in UpdateInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	t, err := h.svc.Update(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"), in)
	var conflict *ConflictError
	if errors.As(err, &conflict) {
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "version conflict", "current": conflict.Current})
		return
	}
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, t)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	var version *int
	if v := r.URL.Query().Get("version"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "version must be a number")
			return
		}
		version = &n
	}
	err := h.svc.Delete(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"), version)
	var conflict *ConflictError
	if errors.As(err, &conflict) {
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "version conflict", "current": conflict.Current})
		return
	}
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

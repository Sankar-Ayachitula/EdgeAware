package sync

import (
	"net/http"
	"strconv"

	"edgeaware/internal/httpx"
	"edgeaware/pkg/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(mux *http.ServeMux, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /sync/pull", protect(http.HandlerFunc(h.pull)))
	mux.Handle("POST /sync/push", protect(http.HandlerFunc(h.push)))
	mux.Handle("GET /users/{id}/devices", protect(http.HandlerFunc(h.devices)))
}

// GET /sync/pull?since=<seq>&limit=<n>&device_id=<uuid>
func (h *Handler) pull(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	since, err := intParam(q.Get("since"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "since must be a number")
		return
	}
	limit, err := intParam(q.Get("limit"))
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "limit must be a number")
		return
	}
	res, err := h.svc.Pull(r.Context(), middleware.UserID(r.Context()), q.Get("device_id"), since, int(limit))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) push(w http.ResponseWriter, r *http.Request) {
	var req PushRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	results, err := h.svc.Push(r.Context(), middleware.UserID(r.Context()), req)
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"results": results})
}

func (h *Handler) devices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.Devices(r.Context(), middleware.UserID(r.Context()), r.PathValue("id"))
	if err != nil {
		httpx.FromError(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, devices)
}

func intParam(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

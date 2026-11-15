// Package task handles creating, viewing and editing tasks, with access decided by
// the hierarchy: you see tasks of yourself and everyone under you, and you edit the
// tasks of people under you. Assignees may only change the status of their own tasks.
package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"edgeaware/internal/apperr"
	"edgeaware/internal/feed"
	"edgeaware/internal/models"
	"edgeaware/internal/policy"
)

type Service struct {
	db  *gorm.DB
	pol *policy.Policy
}

func NewService(db *gorm.DB, pol *policy.Policy) *Service {
	return &Service{db: db, pol: pol}
}

// WithTx returns a copy of the service that runs inside tx (used by sync push so
// the task change and the op record commit together).
func (s *Service) WithTx(tx *gorm.DB) *Service {
	return &Service{db: tx, pol: s.pol}
}

// ConflictError is returned when the client's version is stale; Current is the
// server's copy so the client can merge or retry.
type ConflictError struct{ Current *models.Task }

func (e *ConflictError) Error() string { return "version conflict" }
func (e *ConflictError) Unwrap() error { return apperr.ErrConflict }

// ListForUser returns the tasks assigned to userID, if meID may see that user.
func (s *Service) ListForUser(ctx context.Context, meID, userID string, status string) ([]models.Task, error) {
	if !models.ValidID(userID) {
		return nil, apperr.ErrNotFound
	}
	if err := s.require(s.pol.CanView(ctx, meID, userID)); err != nil {
		return nil, err
	}
	q := s.db.WithContext(ctx).Where("assigned_to = ?", userID)
	if status != "" {
		if !models.TaskStatus(status).Valid() {
			return nil, apperr.Invalid("unknown status %q", status)
		}
		q = q.Where("status = ?", status)
	}
	tasks := []models.Task{}
	err := q.Order("due_at NULLS LAST, created_at").Find(&tasks).Error
	return tasks, err
}

func (s *Service) Get(ctx context.Context, meID, taskID string) (*models.Task, error) {
	t, err := s.load(s.db.WithContext(ctx), taskID)
	if err != nil {
		return nil, err
	}
	if err := s.require(s.pol.CanView(ctx, meID, t.AssignedTo)); err != nil {
		return nil, err
	}
	return t, nil
}

type CreateInput struct {
	ID          string     `json:"id"` // optional; set by offline clients
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueAt       *time.Time `json:"due_at"`
	AssignedTo  string     `json:"assigned_to"`
}

func (s *Service) Create(ctx context.Context, meID string, in CreateInput) (*models.Task, error) {
	in.Title = strings.TrimSpace(in.Title)
	if in.Title == "" {
		return nil, apperr.Invalid("title is required")
	}
	if in.AssignedTo == "" {
		in.AssignedTo = meID
	}
	if !models.ValidID(in.AssignedTo) {
		return nil, apperr.Invalid("assigned_to must be a UUID")
	}
	if in.ID != "" && !models.ValidID(in.ID) {
		return nil, apperr.Invalid("id must be a UUID")
	}
	if err := s.require(s.pol.CanAssignTo(ctx, meID, in.AssignedTo)); err != nil {
		return nil, err
	}

	t := models.Task{
		ID:          in.ID,
		Title:       in.Title,
		Description: in.Description,
		Status:      models.StatusTodo,
		DueAt:       in.DueAt,
		AssignedTo:  in.AssignedTo,
		CreatedBy:   meID,
		Version:     1,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if t.ID != "" {
			// Include soft-deleted rows: an ID can never be reused.
			var n int64
			if err := tx.Unscoped().Model(&models.Task{}).Where("id = ?", t.ID).Count(&n).Error; err != nil {
				return err
			}
			if n > 0 {
				return apperr.Conflict("a task with this id already exists")
			}
		}
		if err := tx.Create(&t).Error; err != nil {
			return err
		}
		return feed.TaskChanged(tx, t.ID, "", t.AssignedTo)
	})
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateInput is a partial update; omitted fields are left unchanged.
// Version must equal the task's current version (optimistic locking).
type UpdateInput struct {
	Version     int                 `json:"version"`
	Title       *string             `json:"title"`
	Description *string             `json:"description"`
	Status      *models.TaskStatus  `json:"status"`
	DueAt       Optional[time.Time] `json:"due_at"`
	AssignedTo  *string             `json:"assigned_to"`
}

func (s *Service) Update(ctx context.Context, meID, taskID string, in UpdateInput) (*models.Task, error) {
	var out *models.Task
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		t, err := s.load(tx, taskID)
		if err != nil {
			return err
		}
		full, statusOnly, err := s.editRights(ctx, meID, t)
		if err != nil {
			return err
		}
		if !full && !statusOnly {
			return s.hiddenOrForbidden(ctx, meID, t)
		}
		if !full && (in.Title != nil || in.Description != nil || in.DueAt.Set || in.AssignedTo != nil) {
			return apperr.ErrForbidden // assignees may only change status
		}
		if in.Version != t.Version {
			return &ConflictError{Current: t}
		}

		updates := map[string]any{}
		if in.Title != nil {
			title := strings.TrimSpace(*in.Title)
			if title == "" {
				return apperr.Invalid("title cannot be empty")
			}
			updates["title"] = title
		}
		if in.Description != nil {
			updates["description"] = *in.Description
		}
		if in.DueAt.Set {
			updates["due_at"] = in.DueAt.Value
		}
		if in.AssignedTo != nil && *in.AssignedTo != t.AssignedTo {
			if !models.ValidID(*in.AssignedTo) {
				return apperr.Invalid("assigned_to must be a UUID")
			}
			if err := s.require(s.pol.CanAssignTo(ctx, meID, *in.AssignedTo)); err != nil {
				return err
			}
			updates["assigned_to"] = *in.AssignedTo
		}
		if in.Status != nil && *in.Status != t.Status {
			if !in.Status.Valid() {
				return apperr.Invalid("unknown status %q", *in.Status)
			}
			updates["status"] = *in.Status
			if *in.Status == models.StatusDone {
				updates["completed_at"] = time.Now()
			} else {
				updates["completed_at"] = nil
			}
		}
		if len(updates) == 0 {
			out = t
			return nil
		}
		updates["version"] = t.Version + 1

		// The version condition protects against a concurrent writer between load and save.
		res := tx.Model(&models.Task{}).Where("id = ? AND version = ?", t.ID, t.Version).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			cur, err := s.load(tx, taskID)
			if err != nil {
				return err
			}
			return &ConflictError{Current: cur}
		}
		out, err = s.load(tx, taskID)
		if err != nil {
			return err
		}
		return feed.TaskChanged(tx, out.ID, t.AssignedTo, out.AssignedTo)
	})
	return out, err
}

// Delete soft-deletes a task (kept as a tombstone so devices can sync the removal).
// If version is non-nil it must match the current version, so an offline device
// cannot delete a task that someone else changed since it last synced.
func (s *Service) Delete(ctx context.Context, meID, taskID string, version *int) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		t, err := s.load(tx, taskID)
		if err != nil {
			return err
		}
		full, _, err := s.editRights(ctx, meID, t)
		if err != nil {
			return err
		}
		if !full {
			return s.hiddenOrForbidden(ctx, meID, t)
		}
		if version != nil && *version != t.Version {
			return &ConflictError{Current: t}
		}
		if err := tx.Model(&models.Task{}).Where("id = ?", t.ID).Update("version", t.Version+1).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.Task{}, "id = ?", t.ID).Error; err != nil {
			return err
		}
		return feed.TaskDeleted(tx, t.ID, t.AssignedTo)
	})
}

// editRights: full edit if the assignee is below me, or it is my own self-created task.
// Status-only if the task is assigned to me but created by someone above me.
func (s *Service) editRights(ctx context.Context, meID string, t *models.Task) (full, statusOnly bool, err error) {
	if t.AssignedTo == meID {
		return t.CreatedBy == meID, t.CreatedBy != meID, nil
	}
	full, err = s.pol.CanManage(ctx, meID, t.AssignedTo)
	return full, false, err
}

// hiddenOrForbidden returns 404 for tasks the user may not even see, 403 otherwise,
// so people cannot probe for tasks outside their branch.
func (s *Service) hiddenOrForbidden(ctx context.Context, meID string, t *models.Task) error {
	ok, err := s.pol.CanView(ctx, meID, t.AssignedTo)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrNotFound
	}
	return apperr.ErrForbidden
}

func (s *Service) load(tx *gorm.DB, taskID string) (*models.Task, error) {
	if !models.ValidID(taskID) {
		return nil, apperr.ErrNotFound
	}
	var t models.Task
	if err := tx.First(&t, "id = ?", taskID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrNotFound
		}
		return nil, err
	}
	return &t, nil
}

func (s *Service) require(ok bool, err error) error {
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}
	return nil
}

// Optional distinguishes "field omitted" from "field set to null" in JSON patches.
type Optional[T any] struct {
	Set   bool
	Value *T
}

func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.Value = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("invalid value: %w", err)
	}
	o.Value = &v
	return nil
}

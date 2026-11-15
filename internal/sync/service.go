// Package sync lets offline devices catch up (pull) and upload their changes (push).
//
// Pull: the device sends the last seq it saw and gets every feed entry after it, in
// order, with the current copy of each task. Push: the device sends a batch of
// changes, each with a unique op_id and the task version it was based on; each
// change is applied, rejected as a conflict, or rejected as forbidden/invalid, and
// the outcome is stored so a retried push returns the same answer.
package sync

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"edgeaware/internal/apperr"
	"edgeaware/internal/models"
	"edgeaware/internal/policy"
	"edgeaware/internal/task"
)

const (
	DefaultPullLimit = 500
	MaxPullLimit     = 1000
	MaxPushChanges   = 500
)

type Service struct {
	db    *gorm.DB
	pol   *policy.Policy
	tasks *task.Service
}

func NewService(db *gorm.DB, pol *policy.Policy, tasks *task.Service) *Service {
	return &Service{db: db, pol: pol, tasks: tasks}
}

// ---------- devices ----------

// touchDevice registers the device on first use and updates last_seen_at. A device
// ID belongs to one user forever; using another user's device ID is forbidden.
func (s *Service) touchDevice(ctx context.Context, meID, deviceID, name string) (*models.Device, error) {
	if !models.ValidID(deviceID) {
		return nil, apperr.Invalid("device_id must be a UUID")
	}
	now := time.Now()
	var d models.Device
	err := s.db.WithContext(ctx).First(&d, "id = ?", deviceID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		d = models.Device{ID: deviceID, UserID: meID, Name: name, LastSeenAt: now}
		return &d, s.db.WithContext(ctx).Create(&d).Error
	case err != nil:
		return nil, err
	case d.UserID != meID:
		return nil, apperr.ErrForbidden
	}
	updates := map[string]any{"last_seen_at": now}
	if name != "" {
		updates["name"] = name
	}
	return &d, s.db.WithContext(ctx).Model(&d).Updates(updates).Error
}

// Devices lists a user's devices, if meID may see that user. Managers can use
// last_seen_at / last_pull_seq to spot devices that have not synced recently.
func (s *Service) Devices(ctx context.Context, meID, userID string) ([]models.Device, error) {
	if !models.ValidID(userID) {
		return nil, apperr.ErrNotFound
	}
	ok, err := s.pol.CanView(ctx, meID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.ErrForbidden
	}
	devices := []models.Device{}
	err = s.db.WithContext(ctx).Where("user_id = ?", userID).Order("last_seen_at DESC").Find(&devices).Error
	return devices, err
}

// ---------- pull ----------

type Change struct {
	Seq    int64        `json:"seq"`
	Kind   string       `json:"kind"` // upsert | deleted | revoked
	TaskID string       `json:"task_id"`
	Task   *models.Task `json:"task,omitempty"` // present for upsert only
}

type PullResult struct {
	Changes    []Change `json:"changes"`
	NextCursor int64    `json:"next_cursor"`
	HasMore    bool     `json:"has_more"`
}

// Pull returns feed entries after `since` for the logged-in user. If a task appears
// several times in one page, only its latest entry is returned. The task copy is the
// current one; if the user can no longer see the task, the entry is sent as revoked.
func (s *Service) Pull(ctx context.Context, meID, deviceID string, since int64, limit int) (*PullResult, error) {
	if limit <= 0 {
		limit = DefaultPullLimit
	}
	if limit > MaxPullLimit {
		limit = MaxPullLimit
	}
	if since < 0 {
		return nil, apperr.Invalid("since must be >= 0")
	}
	if deviceID != "" {
		if _, err := s.touchDevice(ctx, meID, deviceID, ""); err != nil {
			return nil, err
		}
	}

	var rows []models.SyncFeed
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND seq > ?", meID, since).
		Order("seq").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	res := &PullResult{Changes: []Change{}, NextCursor: since}
	if len(rows) > limit {
		res.HasMore = true
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		return res, s.saveCursor(ctx, deviceID, since)
	}
	res.NextCursor = rows[len(rows)-1].Seq

	// Keep only the latest entry per task, preserving seq order.
	latest := map[string]int{}
	for i, r := range rows {
		latest[r.TaskID] = i
	}
	var ids []string
	for i, r := range rows {
		if latest[r.TaskID] == i && r.Kind == models.FeedUpsert {
			ids = append(ids, r.TaskID)
		}
	}

	// Load the upserted tasks that the user can still see (assignee is me or below me).
	visible := map[string]*models.Task{}
	if len(ids) > 0 {
		var tasks []models.Task
		if err := s.db.WithContext(ctx).
			Where("tasks.id IN ?", ids).
			Where("EXISTS (SELECT 1 FROM user_hierarchy h WHERE h.ancestor_id = ? AND h.descendant_id = tasks.assigned_to)", meID).
			Find(&tasks).Error; err != nil {
			return nil, err
		}
		for i := range tasks {
			visible[tasks[i].ID] = &tasks[i]
		}
	}

	for i, r := range rows {
		if latest[r.TaskID] != i {
			continue
		}
		c := Change{Seq: r.Seq, Kind: r.Kind, TaskID: r.TaskID}
		if r.Kind == models.FeedUpsert {
			if t, ok := visible[r.TaskID]; ok {
				c.Task = t
			} else {
				// Deleted or moved out of reach after this entry was written; a later
				// entry will say so too, but removing it now is always safe.
				c.Kind = models.FeedRevoked
			}
		}
		res.Changes = append(res.Changes, c)
	}
	return res, s.saveCursor(ctx, deviceID, res.NextCursor)
}

func (s *Service) saveCursor(ctx context.Context, deviceID string, seq int64) error {
	if deviceID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Model(&models.Device{}).
		Where("id = ? AND last_pull_seq < ?", deviceID, seq).
		Update("last_pull_seq", seq).Error
}

// ---------- push ----------

type PushRequest struct {
	DeviceID   string       `json:"device_id"`
	DeviceName string       `json:"device_name"`
	Changes    []PushChange `json:"changes"`
}

// PushChange is one offline edit. BaseVersion 0 with op "upsert" means "create";
// otherwise it is the task version the device last saw.
type PushChange struct {
	OpID        string          `json:"op_id"`
	Op          string          `json:"op"` // upsert | delete
	TaskID      string          `json:"task_id"`
	BaseVersion int             `json:"base_version"`
	Fields      json.RawMessage `json:"fields"`
}

const (
	StatusApplied   = "applied"
	StatusConflict  = "conflict"
	StatusForbidden = "forbidden"
	StatusNotFound  = "not_found"
	StatusInvalid   = "invalid"
)

type OpResult struct {
	OpID      string       `json:"op_id"`
	Status    string       `json:"status"`
	Error     string       `json:"error,omitempty"`
	Task      *models.Task `json:"task,omitempty"` // the result, or the server copy on conflict
	Duplicate bool         `json:"duplicate,omitempty"`
}

// Push applies each change in order. Each change commits on its own, together with
// its op record, so one bad change does not block the rest of the batch.
func (s *Service) Push(ctx context.Context, meID string, req PushRequest) ([]OpResult, error) {
	if len(req.Changes) == 0 {
		return []OpResult{}, nil
	}
	if len(req.Changes) > MaxPushChanges {
		return nil, apperr.Invalid("at most %d changes per push", MaxPushChanges)
	}
	if _, err := s.touchDevice(ctx, meID, req.DeviceID, req.DeviceName); err != nil {
		return nil, err
	}

	results := make([]OpResult, 0, len(req.Changes))
	for _, c := range req.Changes {
		if c.OpID == "" || len(c.OpID) > 100 {
			results = append(results, OpResult{OpID: c.OpID, Status: StatusInvalid, Error: "op_id is required (max 100 chars)"})
			continue
		}
		r, err := s.pushOne(ctx, meID, req.DeviceID, c)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *Service) pushOne(ctx context.Context, meID, deviceID string, c PushChange) (OpResult, error) {
	var out OpResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Retried op? Return what we answered the first time.
		var prev models.ProcessedOp
		err := tx.Where("device_id = ? AND op_id = ?", deviceID, c.OpID).Take(&prev).Error
		if err == nil {
			if err := json.Unmarshal(prev.Result, &out); err != nil {
				return err
			}
			out.Duplicate = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		out, err = s.apply(ctx, s.tasks.WithTx(tx), meID, c)
		if err != nil {
			return err
		}
		blob, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Create(&models.ProcessedOp{DeviceID: deviceID, OpID: c.OpID, UserID: meID, Result: blob}).Error
	})
	return out, err
}

// apply runs one change and turns expected failures into a result. Only unexpected
// (server) errors are returned as errors, which aborts the push without recording.
func (s *Service) apply(ctx context.Context, tasks *task.Service, meID string, c PushChange) (OpResult, error) {
	res := OpResult{OpID: c.OpID}
	var t *models.Task
	var err error

	switch c.Op {
	case "upsert":
		if c.BaseVersion == 0 {
			var in task.CreateInput
			if err := decodeFields(c.Fields, &in); err != nil {
				return invalid(res, err.Error()), nil
			}
			in.ID = c.TaskID
			if in.ID == "" {
				return invalid(res, "task_id is required"), nil
			}
			t, err = tasks.Create(ctx, meID, in)
			if errors.Is(err, apperr.ErrConflict) {
				// The ID exists already: report the server copy if this user may see it.
				if cur, gerr := tasks.Get(ctx, meID, c.TaskID); gerr == nil {
					err = &task.ConflictError{Current: cur}
				}
			}
		} else {
			var in task.UpdateInput
			if err := decodeFields(c.Fields, &in); err != nil {
				return invalid(res, err.Error()), nil
			}
			in.Version = c.BaseVersion
			t, err = tasks.Update(ctx, meID, c.TaskID, in)
		}
	case "delete":
		v := c.BaseVersion
		err = tasks.Delete(ctx, meID, c.TaskID, &v)
	default:
		return invalid(res, `op must be "upsert" or "delete"`), nil
	}

	var conflict *task.ConflictError
	var ve apperr.Validation
	switch {
	case err == nil:
		res.Status, res.Task = StatusApplied, t
	case errors.As(err, &conflict):
		res.Status, res.Task, res.Error = StatusConflict, conflict.Current, "task changed on the server since base_version"
	case errors.As(err, &ve):
		return invalid(res, ve.Msg), nil
	case errors.Is(err, apperr.ErrConflict):
		res.Status, res.Error = StatusConflict, err.Error()
	case errors.Is(err, apperr.ErrForbidden):
		res.Status = StatusForbidden
	case errors.Is(err, apperr.ErrNotFound):
		res.Status = StatusNotFound
	default:
		return res, err
	}
	return res, nil
}

func decodeFields(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return errors.New("invalid fields: " + err.Error())
	}
	return nil
}

func invalid(res OpResult, msg string) OpResult {
	res.Status, res.Error = StatusInvalid, msg
	return res
}

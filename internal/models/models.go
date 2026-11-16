// Package models holds the GORM models shared by all modules.
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Role is a level in the organisation. Level 0 is the base (field operators);
// higher numbers are more senior. A user's manager must always have a higher level.
type Role struct {
	ID    uint   `gorm:"primaryKey" json:"id"`
	Name  string `gorm:"uniqueIndex;not null" json:"name"`
	Level int    `gorm:"uniqueIndex;not null" json:"level"`
}

// User is anyone who can log in. ManagerID is the direct boss (nil only for the top of the tree)
// and is the source of truth for the hierarchy; UserHierarchy is derived from it.
type User struct {
	ID           string    `gorm:"type:uuid;primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	RoleID       uint      `gorm:"not null" json:"role_id"`
	Role         Role      `json:"role"`
	ManagerID    *string   `gorm:"type:uuid;index" json:"manager_id"`
	Manager      *User     `gorm:"foreignKey:ManagerID" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (u *User) BeforeCreate(*gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	return nil
}

// UserHierarchy is a closure table: one row for every (senior, junior) pair at every
// distance, plus a depth-0 row for each user pointing at themselves.
type UserHierarchy struct {
	AncestorID   string `gorm:"type:uuid;primaryKey"`
	DescendantID string `gorm:"type:uuid;primaryKey;index"`
	Depth        int    `gorm:"not null"`
	Ancestor     User   `gorm:"foreignKey:AncestorID;constraint:OnDelete:CASCADE"`
	Descendant   User   `gorm:"foreignKey:DescendantID;constraint:OnDelete:CASCADE"`
}

func (UserHierarchy) TableName() string { return "user_hierarchy" }

type TaskStatus string

const (
	StatusTodo       TaskStatus = "todo"
	StatusInProgress TaskStatus = "in_progress"
	StatusDone       TaskStatus = "done"
)

func (s TaskStatus) Valid() bool {
	switch s {
	case StatusTodo, StatusInProgress, StatusDone:
		return true
	}
	return false
}

// Task is a unit of work assigned to one user. IDs may be generated on the device so
// tasks can be created offline. Version increases on every change (used for sync conflicts).
type Task struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	Title       string         `gorm:"not null" json:"title"`
	Description string         `json:"description"`
	Status      TaskStatus     `gorm:"type:text;not null;default:todo;check:status IN ('todo','in_progress','done')" json:"status"`
	DueAt       *time.Time     `json:"due_at"`
	CompletedAt *time.Time     `json:"completed_at"`
	AssignedTo  string         `gorm:"type:uuid;not null;index" json:"assigned_to"`
	CreatedBy   string         `gorm:"type:uuid;not null" json:"created_by"`
	Assignee    User           `gorm:"foreignKey:AssignedTo" json:"-"`
	Creator     User           `gorm:"foreignKey:CreatedBy" json:"-"`
	Version     int            `gorm:"not null;default:1" json:"version"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (t *Task) BeforeCreate(*gorm.DB) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	return nil
}

// ValidID reports whether s is a well-formed UUID. Check path/body IDs with it
// before querying, so bad input is a 404/400 rather than a database error.
func ValidID(s string) bool {
	return uuid.Validate(s) == nil
}

// Sync feed kinds. A device applies them in seq order: upsert = store the task,
// deleted / revoked = remove it locally (deleted: task is gone; revoked: you lost access).
const (
	FeedUpsert  = "upsert"
	FeedDeleted = "deleted"
	FeedRevoked = "revoked"
)

// SyncFeed is a per-user change log. Every task change adds one row for each user
// who can see the task (or just lost access to it). Seq comes from one global
// sequence, so a device only needs to remember the last seq it pulled.
type SyncFeed struct {
	UserID    string    `gorm:"type:uuid;primaryKey"`
	Seq       int64     `gorm:"primaryKey"`
	TaskID    string    `gorm:"type:uuid;not null"`
	Kind      string    `gorm:"type:text;not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

func (SyncFeed) TableName() string { return "sync_feed" }

// Device is a phone/tablet that syncs. ID is generated on the device.
type Device struct {
	ID          string    `gorm:"type:uuid;primaryKey" json:"id"`
	UserID      string    `gorm:"type:uuid;not null;index" json:"user_id"`
	Name        string    `json:"name"`
	LastSeenAt  time.Time `json:"last_seen_at"`
	LastPullSeq int64     `json:"last_pull_seq"`
	CreatedAt   time.Time `json:"created_at"`
}

// ProcessedOp records each pushed change by (device, op_id) so a retried push
// returns the original result instead of applying the change twice.
type ProcessedOp struct {
	DeviceID  string    `gorm:"type:uuid;primaryKey"`
	OpID      string    `gorm:"primaryKey"`
	UserID    string    `gorm:"type:uuid;not null"`
	Result    []byte    `gorm:"type:jsonb;not null"`
	CreatedAt time.Time `gorm:"not null;default:now()"`
}

// SyncWatermark is the highest feed seq the cleanup job has deleted for a user.
// A device asking for changes after an older cursor has missed deleted entries and
// must download a fresh snapshot instead.
type SyncWatermark struct {
	UserID        string `gorm:"type:uuid;primaryKey"`
	PrunedThrough int64  `gorm:"not null"`
}

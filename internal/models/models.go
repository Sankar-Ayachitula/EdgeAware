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

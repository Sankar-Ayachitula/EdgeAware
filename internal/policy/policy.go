// Package policy is the single place that decides who may see or change what.
//
// The rule underneath everything: you can act on someone only if they are below
// you in the hierarchy. Comparing role levels alone is not enough (a senior in
// another branch has a higher level but no access), so every check goes through
// the closure table. The logged-in user always comes from the verified token,
// never from the request body.
package policy

import (
	"context"

	"gorm.io/gorm"

	"edgeaware/internal/hierarchy"
	"edgeaware/internal/models"
)

type Policy struct {
	db  *gorm.DB
	hie *hierarchy.Service
}

func New(db *gorm.DB, hie *hierarchy.Service) *Policy {
	return &Policy{db: db, hie: hie}
}

// CanView: yourself, or anyone below you. Never your seniors or another branch.
func (p *Policy) CanView(ctx context.Context, meID, targetID string) (bool, error) {
	if meID == targetID {
		return true, nil
	}
	return p.hie.IsUnder(ctx, meID, targetID)
}

// CanManage: strictly below you. Lets you edit their tasks or move them.
func (p *Policy) CanManage(ctx context.Context, meID, targetID string) (bool, error) {
	return p.hie.IsUnder(ctx, meID, targetID)
}

// CanAssignTo: you may assign a task to anyone below you, or to yourself if you
// are above the base level. Base-level users (level 0) cannot create tasks.
func (p *Policy) CanAssignTo(ctx context.Context, meID, assigneeID string) (bool, error) {
	if meID == assigneeID {
		level, err := p.Level(ctx, meID)
		return level > 0, err
	}
	return p.hie.IsUnder(ctx, meID, assigneeID)
}

// CanAddUserUnder: you may add a new user whose manager is you or anyone below you.
// (The service separately checks the new user's level is below their manager's.)
func (p *Policy) CanAddUserUnder(ctx context.Context, meID, managerID string) (bool, error) {
	return p.CanView(ctx, meID, managerID)
}

// Level returns the role level of a user.
func (p *Policy) Level(ctx context.Context, userID string) (int, error) {
	var level int
	err := p.db.WithContext(ctx).
		Model(&models.User{}).
		Select("roles.level").
		Joins("JOIN roles ON roles.id = users.role_id").
		Where("users.id = ?", userID).
		Scan(&level).Error
	return level, err
}

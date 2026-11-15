// Package hierarchy maintains the user_hierarchy closure table and answers
// "who is under whom" questions.
//
// users.manager_id is the source of truth; every change to it must go through
// this package so the closure table stays in step.
package hierarchy

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"edgeaware/internal/apperr"
	"edgeaware/internal/models"
)

type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

// AddUser inserts the closure rows for a newly created user. Call it inside the
// same transaction that created the user. managerID nil means a top-level user.
func AddUser(tx *gorm.DB, userID string, managerID *string) error {
	if managerID == nil {
		return tx.Exec(
			`INSERT INTO user_hierarchy (ancestor_id, descendant_id, depth) VALUES (?, ?, 0)`,
			userID, userID,
		).Error
	}
	// Every ancestor of the manager (including the manager at depth 0) becomes an
	// ancestor of the new user, one level further away; plus the user's self-row.
	return tx.Exec(`
		INSERT INTO user_hierarchy (ancestor_id, descendant_id, depth)
		SELECT ancestor_id, ?::uuid, depth + 1 FROM user_hierarchy WHERE descendant_id = ?::uuid
		UNION ALL
		SELECT ?::uuid, ?::uuid, 0`,
		userID, *managerID, userID, userID,
	).Error
}

// IsUnder reports whether descendantID is strictly below ancestorID (depth >= 1).
func (s *Service) IsUnder(ctx context.Context, ancestorID, descendantID string) (bool, error) {
	return isUnder(s.db.WithContext(ctx), ancestorID, descendantID)
}

func isUnder(tx *gorm.DB, ancestorID, descendantID string) (bool, error) {
	var n int64
	err := tx.Model(&models.UserHierarchy{}).
		Where("ancestor_id = ? AND descendant_id = ? AND depth >= 1", ancestorID, descendantID).
		Count(&n).Error
	return n > 0, err
}

// DirectJuniors returns the users whose direct manager is userID, most senior first.
func (s *Service) DirectJuniors(ctx context.Context, userID string) ([]models.User, error) {
	var users []models.User
	err := s.db.WithContext(ctx).
		Preload("Role").
		Joins("JOIN user_hierarchy h ON h.descendant_id = users.id").
		Joins("JOIN roles r ON r.id = users.role_id").
		Where("h.ancestor_id = ? AND h.depth = 1", userID).
		Order("r.level DESC, users.username").
		Find(&users).Error
	return users, err
}

// AllUnder returns every user below userID at any depth.
func (s *Service) AllUnder(ctx context.Context, userID string) ([]models.User, error) {
	var users []models.User
	err := s.db.WithContext(ctx).
		Preload("Role").
		Joins("JOIN user_hierarchy h ON h.descendant_id = users.id").
		Where("h.ancestor_id = ? AND h.depth >= 1", userID).
		Order("h.depth, users.username").
		Find(&users).Error
	return users, err
}

// MoveUser gives userID (and their whole subtree) a new direct manager.
// It enforces: no loops, and the new manager must have a higher role level.
func (s *Service) MoveUser(ctx context.Context, userID, newManagerID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if userID == newManagerID {
			return apperr.Invalid("a user cannot manage themselves")
		}
		// Moving someone under one of their own juniors would create a loop.
		loop, err := isUnder(tx, userID, newManagerID)
		if err != nil {
			return err
		}
		if loop {
			return apperr.Invalid("new manager is inside this user's own team")
		}

		var user, mgr models.User
		if err := tx.Preload("Role").First(&user, "id = ?", userID).Error; err != nil {
			return notFound(err)
		}
		if err := tx.Preload("Role").First(&mgr, "id = ?", newManagerID).Error; err != nil {
			return notFound(err)
		}
		if mgr.Role.Level <= user.Role.Level {
			return apperr.Invalid("manager's role level must be higher than the user's")
		}

		// 1. Cut the links between the moved subtree and its old ancestors.
		if err := tx.Exec(`
			DELETE FROM user_hierarchy
			WHERE descendant_id IN (SELECT descendant_id FROM user_hierarchy WHERE ancestor_id = ?)
			  AND ancestor_id NOT IN (SELECT descendant_id FROM user_hierarchy WHERE ancestor_id = ?)`,
			userID, userID,
		).Error; err != nil {
			return err
		}
		// 2. Link every ancestor of the new manager to every member of the subtree.
		if err := tx.Exec(`
			INSERT INTO user_hierarchy (ancestor_id, descendant_id, depth)
			SELECT sup.ancestor_id, sub.descendant_id, sup.depth + sub.depth + 1
			FROM user_hierarchy sup CROSS JOIN user_hierarchy sub
			WHERE sup.descendant_id = ? AND sub.ancestor_id = ?`,
			newManagerID, userID,
		).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", userID).Update("manager_id", newManagerID).Error
	})
}

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.ErrNotFound
	}
	return err
}

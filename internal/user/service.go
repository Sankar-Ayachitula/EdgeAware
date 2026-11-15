// Package user handles user accounts and the hierarchy page endpoints.
package user

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"edgeaware/internal/apperr"
	"edgeaware/internal/auth"
	"edgeaware/internal/hierarchy"
	"edgeaware/internal/models"
	"edgeaware/internal/policy"
)

type Service struct {
	db  *gorm.DB
	hie *hierarchy.Service
	pol *policy.Policy
}

func NewService(db *gorm.DB, hie *hierarchy.Service, pol *policy.Policy) *Service {
	return &Service{db: db, hie: hie, pol: pol}
}

func (s *Service) Roles(ctx context.Context) ([]models.Role, error) {
	var roles []models.Role
	err := s.db.WithContext(ctx).Order("level").Find(&roles).Error
	return roles, err
}

// Get returns a user if meID is allowed to see them.
func (s *Service) Get(ctx context.Context, meID, targetID string) (*models.User, error) {
	if err := s.requireView(ctx, meID, targetID); err != nil {
		return nil, err
	}
	var u models.User
	if err := s.db.WithContext(ctx).Preload("Role").First(&u, "id = ?", targetID).Error; err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

// DirectJuniors returns targetID's direct juniors (the tabs on the hierarchy page).
func (s *Service) DirectJuniors(ctx context.Context, meID, targetID string) ([]models.User, error) {
	if err := s.requireView(ctx, meID, targetID); err != nil {
		return nil, err
	}
	return s.hie.DirectJuniors(ctx, targetID)
}

// Team returns everyone under targetID at any depth.
func (s *Service) Team(ctx context.Context, meID, targetID string) ([]models.User, error) {
	if err := s.requireView(ctx, meID, targetID); err != nil {
		return nil, err
	}
	return s.hie.AllUnder(ctx, targetID)
}

type CreateInput struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	RoleID    uint   `json:"role_id"`
	ManagerID string `json:"manager_id"`
}

// Create adds a user under manager_id. The caller must be that manager or above them,
// and the new user's role level must be lower than the manager's.
func (s *Service) Create(ctx context.Context, meID string, in CreateInput) (*models.User, error) {
	in.Username = strings.TrimSpace(in.Username)
	if in.Username == "" || in.ManagerID == "" || in.RoleID == 0 {
		return nil, apperr.Invalid("username, role_id and manager_id are required")
	}
	if !models.ValidID(in.ManagerID) {
		return nil, apperr.Invalid("manager_id must be a UUID")
	}
	if len(in.Password) < 8 {
		return nil, apperr.Invalid("password must be at least 8 characters")
	}
	ok, err := s.pol.CanAddUserUnder(ctx, meID, in.ManagerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperr.ErrForbidden
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	var created models.User
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role models.Role
		if err := tx.First(&role, in.RoleID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return apperr.Invalid("unknown role_id")
			}
			return err
		}
		var mgr models.User
		if err := tx.Preload("Role").First(&mgr, "id = ?", in.ManagerID).Error; err != nil {
			return notFound(err)
		}
		if role.Level >= mgr.Role.Level {
			return apperr.Invalid("new user's role level must be lower than their manager's")
		}

		var taken int64
		if err := tx.Model(&models.User{}).Where("username = ?", in.Username).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return apperr.Conflict("username already taken")
		}

		created = models.User{
			Username:     in.Username,
			PasswordHash: hash,
			RoleID:       role.ID,
			ManagerID:    &mgr.ID,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		created.Role = role
		return hierarchy.AddUser(tx, created.ID, created.ManagerID)
	})
	if err != nil {
		return nil, err
	}
	return &created, nil
}

// ChangeManager moves targetID (and their team) under newManagerID.
// The caller must be above the target, and the new manager must be the caller or below them.
func (s *Service) ChangeManager(ctx context.Context, meID, targetID, newManagerID string) error {
	if !models.ValidID(targetID) {
		return apperr.ErrNotFound
	}
	if !models.ValidID(newManagerID) {
		return apperr.Invalid("manager_id must be a UUID")
	}
	ok, err := s.pol.CanManage(ctx, meID, targetID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}
	ok, err = s.pol.CanView(ctx, meID, newManagerID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}
	return s.hie.MoveUser(ctx, targetID, newManagerID)
}

func (s *Service) requireView(ctx context.Context, meID, targetID string) error {
	if !models.ValidID(targetID) {
		return apperr.ErrNotFound
	}
	ok, err := s.pol.CanView(ctx, meID, targetID)
	if err != nil {
		return err
	}
	if !ok {
		return apperr.ErrForbidden
	}
	return nil
}

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.ErrNotFound
	}
	return err
}

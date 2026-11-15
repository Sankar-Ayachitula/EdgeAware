// Package seed loads a small sample organisation for local development and tests.
package seed

import (
	"fmt"

	"gorm.io/gorm"

	"edgeaware/internal/auth"
	"edgeaware/internal/hierarchy"
	"edgeaware/internal/models"
)

// DevPassword is the password of every seeded user. Development only.
const DevPassword = "edgeaware-dev"

// Roles from the base level (0) up.
var Roles = []models.Role{
	{Name: "Field Operator", Level: 0},
	{Name: "Supervisor", Level: 1},
	{Name: "Manager", Level: 2},
	{Name: "Director", Level: 3},
}

// Org is the sample tree: username -> manager username ("" = top of the tree).
//
//	director
//	├── manager_north
//	│   ├── supervisor_n1 ── operator_n1a, operator_n1b
//	│   └── supervisor_n2 ── operator_n2a
//	└── manager_south
//	    └── supervisor_s1 ── operator_s1a
var Org = []struct{ Username, Role, Manager string }{
	{"director", "Director", ""},
	{"manager_north", "Manager", "director"},
	{"manager_south", "Manager", "director"},
	{"supervisor_n1", "Supervisor", "manager_north"},
	{"supervisor_n2", "Supervisor", "manager_north"},
	{"supervisor_s1", "Supervisor", "manager_south"},
	{"operator_n1a", "Field Operator", "supervisor_n1"},
	{"operator_n1b", "Field Operator", "supervisor_n1"},
	{"operator_n2a", "Field Operator", "supervisor_n2"},
	{"operator_s1a", "Field Operator", "supervisor_s1"},
}

// Run inserts the roles and sample users if the users table is empty. It returns
// the users by username.
func Run(db *gorm.DB) (map[string]models.User, error) {
	users := map[string]models.User{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("users table is not empty; refusing to seed")
		}

		roles := map[string]models.Role{}
		for _, r := range Roles {
			r := r
			if err := tx.Where(models.Role{Name: r.Name}).Attrs(models.Role{Level: r.Level}).FirstOrCreate(&r).Error; err != nil {
				return err
			}
			roles[r.Name] = r
		}

		hash, err := auth.HashPassword(DevPassword)
		if err != nil {
			return err
		}
		for _, o := range Org {
			u := models.User{Username: o.Username, PasswordHash: hash, RoleID: roles[o.Role].ID}
			if o.Manager != "" {
				id := users[o.Manager].ID
				u.ManagerID = &id
			}
			if err := tx.Create(&u).Error; err != nil {
				return err
			}
			if err := hierarchy.AddUser(tx, u.ID, u.ManagerID); err != nil {
				return err
			}
			u.Role = roles[o.Role]
			users[o.Username] = u
		}
		return nil
	})
	return users, err
}

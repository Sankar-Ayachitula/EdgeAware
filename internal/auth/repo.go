package auth

import "database/sql"

type UserRepo struct {
	db *sql.DB
}

func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) FindByUsername(username string) (User, bool, error) {
	var u User
	err := r.db.QueryRow(
		`SELECT id::text, username, password, role FROM users WHERE username=$1`,
		username,
	).Scan(&u.ID, &u.Username, &u.Password, &u.Role)

	if err == sql.ErrNoRows {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, err
	}
	return u, true, nil
}

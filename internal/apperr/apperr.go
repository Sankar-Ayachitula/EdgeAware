// Package apperr defines the errors services return; httpx maps them to status codes.
package apperr

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
)

// Validation is a bad-input error whose message is safe to show to the client.
type Validation struct{ Msg string }

func (v Validation) Error() string { return v.Msg }

func Invalid(format string, args ...any) error {
	return Validation{Msg: fmt.Sprintf(format, args...)}
}

// Conflict wraps ErrConflict with a client-facing message.
func Conflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

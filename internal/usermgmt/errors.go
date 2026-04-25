// Package usermgmt centralizes local user-management rules for RPC, local IPC,
// and offline CLI flows.
package usermgmt

import "errors"

var (
	// ErrUserExists reports that a username is already taken.
	ErrUserExists = errors.New(`user already exists`)
	// ErrDuplicateUsername is an alias for ErrUserExists kept for caller clarity.
	ErrDuplicateUsername = ErrUserExists
	// ErrUserNotFound reports that the requested user does not exist.
	ErrUserNotFound = errors.New(`user not found`)
	// ErrUserNameRequired reports that a username was required but missing.
	ErrUserNameRequired = errors.New(`user_name is required`)
	// ErrEmailRequired reports that an email was required but missing.
	ErrEmailRequired = errors.New(`email is required`)
	// ErrPasswordRequired reports that a create request omitted the password.
	ErrPasswordRequired = errors.New(`password is required`)
	// ErrPasswordEmpty reports that a requested password change was blank.
	ErrPasswordEmpty = errors.New(`password cannot be empty`)
	// ErrUserIDRequired reports that a user ID was required but missing.
	ErrUserIDRequired = errors.New(`id is required`)
	// ErrLastSuperUser reports that a mutation would remove the final superuser.
	ErrLastSuperUser = errors.New(`cannot remove the last super user`)
	// ErrCannotDeleteSelf reports that an RPC-scoped self-delete was attempted.
	ErrCannotDeleteSelf = errors.New(`cannot delete your own user`)
)

// Package usermgmt centralizes local user-management rules for RPC, local IPC,
// and offline CLI flows.
package usermgmt

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// User is the shared user-management view returned by the service layer.
type User struct {
	ID          string     `json:"id"`
	UserName    string     `json:"username"`
	Email       string     `json:"email"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	SuperUser   bool       `json:"superuser"`
	LastLoginAt *time.Time `json:"last_login,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateInput describes a new local user to create.
type CreateInput struct {
	UserName  string `json:"username"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Password  string `json:"password"`
	SuperUser bool   `json:"superuser"`
}

// UpdateInput describes a partial update to an existing local user.
type UpdateInput struct {
	ID        string  `json:"id"`
	UserName  *string `json:"username,omitempty"`
	Email     *string `json:"email,omitempty"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
	Password  *string `json:"password,omitempty"`
	SuperUser *bool   `json:"superuser,omitempty"`
}

// DeleteInput describes a local user deletion request.
type DeleteInput struct {
	ID                string
	ActingUserID      string
	PreventSelfDelete bool
}

// Service applies shared local user-management rules on top of the DB layer.
type Service struct {
	db *db.Connection
}

// NewService creates a shared user-management service.
func NewService(database *db.Connection) *Service {
	return &Service{db: database}
}

// List returns all local users.
func (s *Service) List() ([]*User, error) {
	users, errGetUsers := s.db.GetAllUsers()
	if errGetUsers != nil {
		if errors.Is(errGetUsers, sql.ErrNoRows) {
			return []*User{}, nil
		}
		return nil, fmt.Errorf(`usermgmt: list users: %w`, errGetUsers)
	}

	result := make([]*User, len(users))
	for i, user := range users {
		result[i] = userModelToUser(user)
	}

	return result, nil
}

// GetByID returns a local user by ID.
func (s *Service) GetByID(id string) (*User, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == `` {
		return nil, ErrUserIDRequired
	}

	userModel, errGetUser := s.db.GetUserByID(trimmedID)
	if errGetUser != nil {
		return nil, mapUserLookupError(errGetUser)
	}

	return userModelToUser(userModel), nil
}

// GetByUsername returns a local user by username.
func (s *Service) GetByUsername(username string) (*User, error) {
	trimmedUserName := strings.TrimSpace(username)
	if trimmedUserName == `` {
		return nil, ErrUserNameRequired
	}

	userModel, errGetUser := s.db.GetUser(trimmedUserName)
	if errGetUser != nil {
		return nil, mapUserLookupError(errGetUser)
	}

	return userModelToUser(userModel), nil
}

// Create creates a local user while enforcing shared validation rules.
func (s *Service) Create(input CreateInput) (*User, error) {
	userName := strings.TrimSpace(input.UserName)
	email := strings.TrimSpace(input.Email)
	firstName := strings.TrimSpace(input.FirstName)
	lastName := strings.TrimSpace(input.LastName)
	password := input.Password

	if userName == `` {
		return nil, ErrUserNameRequired
	}
	if email == `` {
		return nil, ErrEmailRequired
	}
	if strings.TrimSpace(password) == `` {
		return nil, ErrPasswordRequired
	}

	passwordHash, errHashPassword := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if errHashPassword != nil {
		log.Error().Err(errHashPassword).Str(`user_name`, userName).Msg(`failed to hash password for user create`)
		return nil, errors.New(`internal error`)
	}

	newID, errID := helpers.GenerateUniqueID()
	if errID != nil {
		log.Error().Err(errID).Str(`user_name`, userName).Msg(`failed to generate user id`)
		return nil, errors.New(`internal error`)
	}

	createdUser, errCreateUser := s.db.CreateUser(&models.UserSetter{
		ID:           omit.From(newID.String()),
		UserName:     omit.From(userName),
		Email:        omit.From(email),
		FirstName:    omit.From(firstName),
		LastName:     omit.From(lastName),
		PasswordHash: omit.From(string(passwordHash)),
		SuperUser:    omit.From(input.SuperUser),
	})
	if errCreateUser != nil {
		if isSQLiteUniqueConstraintError(errCreateUser) {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf(`usermgmt: create user: %w`, errCreateUser)
	}

	return userModelToUser(createdUser), nil
}

// Update applies a partial update to a local user.
func (s *Service) Update(input UpdateInput) (*User, error) {
	userID := strings.TrimSpace(input.ID)
	if userID == `` {
		return nil, ErrUserIDRequired
	}

	targetUser, errGetTargetUser := s.db.GetUserByID(userID)
	if errGetTargetUser != nil {
		return nil, mapUserLookupError(errGetTargetUser)
	}

	nextSuperUser := targetUser.SuperUser
	if input.SuperUser != nil {
		nextSuperUser = *input.SuperUser
	}
	if targetUser.SuperUser && !nextSuperUser {
		superUserCount, errSuperUserCount := s.db.CountSuperUsers()
		if errSuperUserCount != nil {
			return nil, fmt.Errorf(`usermgmt: count super users for update: %w`, errSuperUserCount)
		}
		if superUserCount <= 1 {
			return nil, ErrLastSuperUser
		}
	}

	userSetter := &models.UserSetter{
		ID: omit.From(userID),
	}
	hasChanges := false

	if input.UserName != nil {
		userName := strings.TrimSpace(*input.UserName)
		if userName == `` {
			return nil, ErrUserNameRequired
		}
		userSetter.UserName = omit.From(userName)
		hasChanges = true
	}
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		if email == `` {
			return nil, ErrEmailRequired
		}
		userSetter.Email = omit.From(email)
		hasChanges = true
	}
	if input.FirstName != nil {
		userSetter.FirstName = omit.From(strings.TrimSpace(*input.FirstName))
		hasChanges = true
	}
	if input.LastName != nil {
		userSetter.LastName = omit.From(strings.TrimSpace(*input.LastName))
		hasChanges = true
	}
	if input.SuperUser != nil {
		userSetter.SuperUser = omit.From(*input.SuperUser)
		hasChanges = true
	}
	if input.Password != nil {
		password := *input.Password
		if strings.TrimSpace(password) == `` {
			return nil, ErrPasswordEmpty
		}

		passwordHash, errHashPassword := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if errHashPassword != nil {
			log.Error().Err(errHashPassword).Str(`user_id`, userID).Msg(`failed to hash password for user update`)
			return nil, errors.New(`internal error`)
		}
		userSetter.PasswordHash = omit.From(string(passwordHash))
		hasChanges = true
	}

	if !hasChanges {
		return userModelToUser(targetUser), nil
	}

	userSetter.UpdatedAt = omit.From(time.Now().UTC())
	errUpdateUser := s.db.UpdateUser(userSetter)
	if errUpdateUser != nil {
		if isSQLiteUniqueConstraintError(errUpdateUser) {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf(`usermgmt: update user: %w`, errUpdateUser)
	}

	updatedUser, errGetUpdatedUser := s.db.GetUserByID(userID)
	if errGetUpdatedUser != nil {
		return nil, fmt.Errorf(`usermgmt: get updated user: %w`, errGetUpdatedUser)
	}

	return userModelToUser(updatedUser), nil
}

// Delete removes a local user while enforcing shared guardrails.
func (s *Service) Delete(input DeleteInput) error {
	userID := strings.TrimSpace(input.ID)
	if userID == `` {
		return ErrUserIDRequired
	}

	targetUser, errGetTargetUser := s.db.GetUserByID(userID)
	if errGetTargetUser != nil {
		return mapUserLookupError(errGetTargetUser)
	}

	if targetUser.SuperUser {
		superUserCount, errSuperUserCount := s.db.CountSuperUsers()
		if errSuperUserCount != nil {
			return fmt.Errorf(`usermgmt: count super users for delete: %w`, errSuperUserCount)
		}
		if superUserCount <= 1 {
			return ErrLastSuperUser
		}
	}

	if input.PreventSelfDelete && input.ActingUserID != `` && input.ActingUserID == targetUser.ID {
		return ErrCannotDeleteSelf
	}

	errDeleteUser := s.db.DeleteUser(userID)
	if errDeleteUser != nil {
		if errors.Is(errDeleteUser, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return fmt.Errorf(`usermgmt: delete user: %w`, errDeleteUser)
	}

	return nil
}

func mapUserLookupError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	return fmt.Errorf(`usermgmt: user lookup: %w`, err)
}

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), `unique constraint failed`)
}

func userModelToUser(userModel *models.User) *User {
	var lastLoginAt *time.Time
	if !userModel.LastLoginAt.IsZero() {
		lastLogin := userModel.LastLoginAt
		lastLoginAt = &lastLogin
	}

	return &User{
		ID:          userModel.ID,
		UserName:    userModel.UserName,
		Email:       userModel.Email,
		FirstName:   userModel.FirstName,
		LastName:    userModel.LastName,
		SuperUser:   userModel.SuperUser,
		LastLoginAt: lastLoginAt,
		CreatedAt:   userModel.CreatedAt,
		UpdatedAt:   userModel.UpdatedAt,
	}
}

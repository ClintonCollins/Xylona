// Package firstsetup implements first-run secret generation, setup tokens, and
// creation of the first superuser.
package firstsetup

import (
	"fmt"
	"sync"

	"github.com/ClintonCollins/Xylona/internal/usermgmt"
)

var createFirstSuperUserMu sync.Mutex

// CreateFirstSuperUser creates the first controller superuser. It fails if
// a superuser already exists.
func CreateFirstSuperUser(service *usermgmt.Service, input usermgmt.CreateInput) (*usermgmt.User, error) {
	createFirstSuperUserMu.Lock()
	defer createFirstSuperUserMu.Unlock()

	superUserCount, errCount := service.CountSuperUsers()
	if errCount != nil {
		return nil, fmt.Errorf("firstsetup: count superusers: %w", errCount)
	}
	if superUserCount > 0 {
		return nil, ErrAlreadyInstalled
	}

	input.SuperUser = true
	createdUser, errCreate := service.Create(input)
	if errCreate != nil {
		return nil, fmt.Errorf("firstsetup: create user: %w", errCreate)
	}
	return createdUser, nil
}

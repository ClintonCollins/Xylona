package firstsetup

import (
	"errors"
	"sync"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
)

func TestCreateFirstSuperUser(t *testing.T) {
	t.Run("creates the first user as a superuser", func(t *testing.T) {
		service := newTestUserService(t)
		user, errCreate := CreateFirstSuperUser(service, usermgmt.CreateInput{
			UserName: "admin",
			Email:    "admin@localhost",
			Password: "secret-password",
		})
		if errCreate != nil {
			t.Fatalf("CreateFirstSuperUser() error = %v", errCreate)
		}
		if !user.SuperUser {
			t.Fatal("CreateFirstSuperUser() SuperUser = false, want true")
		}
	})

	t.Run("rejects setup when a superuser already exists", func(t *testing.T) {
		service := newTestUserService(t)
		_, errCreate := CreateFirstSuperUser(service, usermgmt.CreateInput{
			UserName: "admin",
			Email:    "admin@localhost",
			Password: "secret-password",
		})
		if errCreate != nil {
			t.Fatalf("first CreateFirstSuperUser() error = %v", errCreate)
		}

		_, errCreate = CreateFirstSuperUser(service, usermgmt.CreateInput{
			UserName: "other",
			Email:    "other@localhost",
			Password: "secret-password",
		})
		if !errors.Is(errCreate, ErrAlreadyInstalled) {
			t.Fatalf("second CreateFirstSuperUser() error = %v, want %v", errCreate, ErrAlreadyInstalled)
		}
	})

	t.Run("returns user validation errors", func(t *testing.T) {
		service := newTestUserService(t)
		_, errCreate := CreateFirstSuperUser(service, usermgmt.CreateInput{
			Email:    "admin@localhost",
			Password: "secret-password",
		})
		if !errors.Is(errCreate, usermgmt.ErrUserNameRequired) {
			t.Fatalf("CreateFirstSuperUser() error = %v, want %v", errCreate, usermgmt.ErrUserNameRequired)
		}
	})

	t.Run("serializes concurrent calls", func(t *testing.T) {
		service := newTestUserService(t)
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, userName := range []string{"first", "second"} {
			wg.Go(func() {
				<-start
				_, errCreate := CreateFirstSuperUser(service, usermgmt.CreateInput{
					UserName: userName,
					Email:    userName + "@localhost",
					Password: "secret-password",
				})
				results <- errCreate
			})
		}
		close(start)
		wg.Wait()
		close(results)

		successes := 0
		alreadyInstalled := 0
		for errCreate := range results {
			switch {
			case errCreate == nil:
				successes++
			case errors.Is(errCreate, ErrAlreadyInstalled):
				alreadyInstalled++
			default:
				t.Fatalf("CreateFirstSuperUser() error = %v", errCreate)
			}
		}
		if successes != 1 || alreadyInstalled != 1 {
			t.Fatalf("concurrent results = %d successes, %d already installed; want 1 each", successes, alreadyInstalled)
		}
	})
}

func newTestUserService(t *testing.T) *usermgmt.Service {
	t.Helper()
	return usermgmt.NewService(dbtest.NewMigratedConnection(t, "first-setup.sqlite"))
}

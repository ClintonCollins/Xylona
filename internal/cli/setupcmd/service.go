package setupcmd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/adminipc"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/firstsetup"
	"github.com/ClintonCollins/Xylona/internal/usermgmt"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/proto/go/xylona/xylonaconnect"
)

type firstUserCreator interface {
	CreateFirstSuperUser(usermgmt.CreateInput) (*usermgmt.User, error)
}

func openUserService(
	ctx context.Context,
	options Options,
	dbPath string,
	forceOffline bool,
) (firstUserCreator, func() error, error) {
	client, errClient := adminipc.NewClient(dbPath)
	if errClient == nil {
		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		_, errPing := client.ListUsers(pingCtx, connect.NewRequest(&xylona.ListUsersRequest{}))
		if errPing == nil {
			if !forceOffline {
				return &liveUserService{ctx: ctx, client: client}, func() error { return nil }, nil
			}
			return nil, nil, wrapCommandError("open setup service", adminipc.ErrLiveDaemonRunning)
		}
	}

	lock, errLock := adminipc.AcquireAppLock(dbPath)
	if errLock != nil {
		return nil, nil, wrapCommandError(
			"guard offline mutation",
			fmt.Errorf("%w: %w", adminipc.ErrAppLockHeld, errLock),
		)
	}

	errEnsure := ensureOfflineSecrets(dbPath)
	if errEnsure != nil {
		errCloseLock := lock.Close()
		return nil, nil, errors.Join(errEnsure, wrapCommandError("release offline lock", errCloseLock))
	}

	conn, _, errConn := db.OpenOfflineUserConnection(ctx, dbPath)
	if errConn != nil {
		errCloseLock := lock.Close()
		return nil, nil, errors.Join(
			wrapCommandError("open offline database", errConn),
			wrapCommandError("release offline lock", errCloseLock),
		)
	}
	errVerify := db.VerifyOfflineWriteAccess(ctx, conn)
	if errVerify != nil {
		errCloseDB := conn.SQLDb.Close()
		errCloseLock := lock.Close()
		return nil, nil, errors.Join(
			wrapCommandError("verify offline write access", errVerify),
			wrapCommandError("close offline database", errCloseDB),
			wrapCommandError("release offline lock", errCloseLock),
		)
	}
	cleanup := func() error {
		errCloseDB := conn.SQLDb.Close()
		errCloseLock := lock.Close()
		return errors.Join(
			wrapCommandError("close offline database", errCloseDB),
			wrapCommandError("release offline lock", errCloseLock),
		)
	}
	if options.Migrate != nil {
		errMigrate := options.Migrate(conn.SQLDb)
		if errMigrate != nil {
			return nil, nil, errors.Join(wrapCommandError("run migrations", errMigrate), cleanup())
		}
	}

	return &offlineUserService{service: usermgmt.NewService(conn)}, cleanup, nil
}

type offlineUserService struct {
	service *usermgmt.Service
}

func (s *offlineUserService) CreateFirstSuperUser(input usermgmt.CreateInput) (*usermgmt.User, error) {
	user, errCreate := firstsetup.CreateFirstSuperUser(s.service, input)
	if errCreate != nil {
		return nil, wrapCommandError("create first offline superuser", errCreate)
	}
	return user, nil
}

type liveUserService struct {
	ctx    context.Context
	client xylonaconnect.XylonaClient
}

func (s *liveUserService) CreateFirstSuperUser(input usermgmt.CreateInput) (*usermgmt.User, error) {
	response, errCreate := s.client.CompleteSetup(s.ctx, connect.NewRequest(&xylona.CompleteSetupRequest{
		UserName: input.UserName,
		Email:    input.Email,
		Password: input.Password,
	}))
	if errCreate != nil {
		if connect.CodeOf(errCreate) == connect.CodeFailedPrecondition {
			return nil, firstsetup.ErrAlreadyInstalled
		}
		return nil, wrapCommandError("create first live superuser", errCreate)
	}
	return protoUserToUser(response.Msg.GetUser()), nil
}

func protoUserToUser(user *xylona.User) *usermgmt.User {
	if user == nil {
		return nil
	}
	return &usermgmt.User{
		UserName:  user.GetUserName(),
		Email:     user.GetEmail(),
		SuperUser: user.GetSuperUser(),
	}
}

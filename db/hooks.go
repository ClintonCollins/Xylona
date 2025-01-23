package db

import (
	"context"
	"strings"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func beforeInsertUser(ctx context.Context, _ bob.Executor, userSetter *models.UserSetter) (context.Context, error) {
	userSetter.UserName = omit.From(strings.ToLower(userSetter.UserName.GetOrZero()))
	userSetter.ID = omit.From(uuid.NewString())
	return ctx, nil
}

func beforeUpdateUser(ctx context.Context, _ bob.Executor, users models.UserSlice) (context.Context, error) {
	for _, user := range users {
		user.UserName = strings.ToLower(user.UserName)
	}
	return ctx, nil
}

func beforeUpsertUser(ctx context.Context, _ bob.Executor, users models.UserSlice) (context.Context, error) {
	for _, user := range users {
		user.UserName = strings.ToLower(user.UserName)
	}
	return ctx, nil
}

func beforeInsertSession(ctx context.Context, _ bob.Executor, sessionSetter *models.UserSessionSetter) (context.Context, error) {
	sessionSetter.ID = omit.From(uuid.NewString())
	sessionSetter.Token = omit.From(uuid.NewString())
	return ctx, nil
}

package db

import (
	"context"
	"strings"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/stephenafamo/bob"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func beforeInsertUser(ctx context.Context, exec bob.Executor, userSetter []*models.UserSetter) (context.Context, error) {
	for _, user := range userSetter {
		user.UserName = omit.From(strings.ToLower(user.UserName.GetOrZero()))
		user.ID = omit.From(uuid.NewString())
	}
	return ctx, nil
}

func beforeUpdateUser(ctx context.Context, exec bob.Executor, users models.UserSlice) (context.Context, error) {
	for _, user := range users {
		user.UserName = strings.ToLower(user.UserName)
	}
	return ctx, nil
}

func beforeUpsertUser(ctx context.Context, exec bob.Executor, userSetter []*models.UserSetter) (context.Context, error) {
	for _, user := range userSetter {
		user.UserName = omit.From(strings.ToLower(user.UserName.GetOrZero()))
	}
	return ctx, nil
}

func beforeInsertSession(ctx context.Context, exec bob.Executor, sessionSetter []*models.UserSessionSetter) (context.Context, error) {
	for _, session := range sessionSetter {
		session.ID = omit.From(uuid.NewString())
		session.Token = omit.From(uuid.NewString())
	}
	return ctx, nil
}

package rpc

import (
	"context"

	"github.com/gorilla/securecookie"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type XylonaService struct {
	ctx            context.Context
	db             *db.Connection
	actionsInst    *actions.Instance
	supervisorInst *supervisor.Instance
	secureCookie   *securecookie.SecureCookie
}

func NewXylonaService(ctx context.Context, db *db.Connection, actionsInst *actions.Instance, supervisorInst *supervisor.Instance, secureCookie *securecookie.SecureCookie) *XylonaService {
	return &XylonaService{
		ctx:            ctx,
		db:             db,
		actionsInst:    actionsInst,
		secureCookie:   secureCookie,
		supervisorInst: supervisorInst,
	}
}

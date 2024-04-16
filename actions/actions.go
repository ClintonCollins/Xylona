package actions

import (
	"context"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/supervisor"
)

type Instance struct {
	ctx                context.Context
	supervisorInstance *supervisor.Instance
	db                 *db.Connection
}

func NewInstance(ctx context.Context, db *db.Connection, supervisorInstance *supervisor.Instance) *Instance {
	return &Instance{
		ctx:                ctx,
		supervisorInstance: supervisorInstance,
		db:                 db,
	}
}

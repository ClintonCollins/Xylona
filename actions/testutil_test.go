package actions

import (
	"context"
	"testing"

	"github.com/ClintonCollins/Xylona/db/dbtest"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
)

func newTestInstance(t *testing.T) *Instance {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	conn := dbtest.NewMigratedConnection(t, "test-actions.sqlite")
	t.Cleanup(cancel)

	return NewInstance(ctx, conn, nil, nil, nil, versiontracker.NewVersionStateMap(), versiontracker.ResolverConfig{})
}

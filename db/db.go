package db

import (
	"context"
	"database/sql"

	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob"
	_ "modernc.org/sqlite"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type Connection struct {
	ctx   context.Context
	SQLDb *sql.DB
	*bob.DB
}

func setupHooks() {
	models.Users.BeforeInsertHooks.Add(beforeInsertUser)
	models.Users.BeforeUpdateHooks.Add(beforeUpdateUser)
	models.Users.BeforeUpsertHooks.Add(beforeUpsertUser)
	models.UserSessions.BeforeInsertHooks.Add(beforeInsertSession)
}

func NewConnection(ctx context.Context, path string) *Connection {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Fatal().Err(err).Msg("Error connecting to database")
	}
	err = db.PingContext(context.Background())
	if err != nil {
		log.Fatal().Err(err).Msg("Error pinging database")
	}
	bobDB := bob.NewDB(db)
	setupHooks()
	return &Connection{ctx, db, &bobDB}
}

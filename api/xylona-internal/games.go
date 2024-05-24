package internal

import (
	"io"
	"sync"

	"github.com/ClintonCollins/Xylona/sql/models"
)

var (
	games     = map[string]Game{}
	gamesLock = &sync.RWMutex{}
)

type Game interface {
	Install(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error
	Update(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error
}

func RegisterGame(id string, game Game) {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	games[id] = game
}

func GetGame(id string) (Game, bool) {
	gamesLock.RLock()
	defer gamesLock.RUnlock()
	game, exists := games[id]
	return game, exists
}

func GetGames() map[string]Game {
	gamesLock.RLock()
	defer gamesLock.RUnlock()
	return games
}

// Package internal registers built-in game installers and updaters.
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

// Game defines install and update hooks for an internal game integration.
type Game interface {
	Install(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error
	Update(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error
}

// RegisterGame registers an internal game integration by ID.
func RegisterGame(id string, game Game) {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	games[id] = game
}

// GetGame returns a registered internal game integration by ID.
func GetGame(id string) (Game, bool) {
	gamesLock.RLock()
	defer gamesLock.RUnlock()
	game, exists := games[id]
	return game, exists
}

// GetGames returns the registered internal game integrations.
func GetGames() map[string]Game {
	gamesLock.RLock()
	defer gamesLock.RUnlock()
	return games
}

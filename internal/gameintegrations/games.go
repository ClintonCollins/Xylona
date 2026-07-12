// Package gameintegrations registers built-in game installers and updaters.
package gameintegrations

import (
	"io"
	"maps"
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

// EnvironmentUpdater is implemented by internal updaters that need ephemeral
// launch credentials. Values are provided only for the duration of Update and
// must not be persisted or logged.
type EnvironmentUpdater interface {
	UpdateWithEnvironment(
		gameServer *models.GameServer,
		stdOutWriter io.Writer,
		stdErrWriter io.Writer,
		environment map[string]string,
	) error
}

// RegisterGame registers an internal game integration by ID.
func RegisterGame(id string, game Game) {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	games[id] = game
}

// UnregisterGameForTest removes a registered internal game integration.
func UnregisterGameForTest(id string) {
	gamesLock.Lock()
	defer gamesLock.Unlock()
	delete(games, id)
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
	result := make(map[string]Game, len(games))
	maps.Copy(result, games)
	return result
}

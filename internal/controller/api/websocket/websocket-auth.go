package websocket

import (
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func (c *connection) revalidateSession() error {
	c.RLock()
	lookup := c.sessionUserLookup
	userID := c.userID
	c.RUnlock()
	return c.validateSessionUser(lookup, userID)
}

func (c *connection) validateSession() error {
	c.RLock()
	lookup := c.sessionUserValidation
	userID := c.userID
	c.RUnlock()
	return c.validateSessionUser(lookup, userID)
}

func (c *connection) validateSessionUser(lookup func() (*models.User, error), userID string) error {
	if lookup == nil {
		return errors.New("websocket session lookup is unavailable")
	}

	user, errLookup := lookup()
	if errLookup != nil {
		return errLookup
	}
	if user == nil || user.ID != userID {
		return errors.New("websocket session user does not match connection user")
	}

	c.Lock()
	c.isSuperUser = user.SuperUser
	c.lastSuperUserCheck = time.Now()
	c.Unlock()
	return nil
}

func getSessionUsername(s *melody.Session) (string, error) {
	u, usernameExists := s.Get(sessionKeyUserName)
	if !usernameExists {
		return "", errSessionUsernameMissing
	}
	username, ok := u.(string)
	if !ok {
		return "", errSessionUsernameMissing
	}
	return username, nil
}

func getSessionUserID(s *melody.Session) (string, error) {
	u, userIDExists := s.Get(sessionKeyUserID)
	if !userIDExists {
		return "", errSessionUserIDMissing
	}
	userID, ok := u.(string)
	if !ok {
		return "", errSessionUserIDMissing
	}
	return userID, nil
}

func getSessionConnectionID(s *melody.Session) (uuid.UUID, error) {
	c, connectionIDExists := s.Get(sessionKeyConnectionID)
	if !connectionIDExists {
		return uuid.Nil, errSessionConnectionIDMissing
	}
	connectionID, ok := c.(uuid.UUID)
	if !ok {
		return uuid.Nil, errSessionConnectionIDMissing
	}
	return connectionID, nil
}

func hasSessionConnectionState(s *melody.Session) bool {
	_, userIDExists := s.Get(sessionKeyUserID)
	if !userIDExists {
		return false
	}
	_, connectionIDExists := s.Get(sessionKeyConnectionID)
	return connectionIDExists
}

// shouldReceiveMetrics returns true if the connection is subscribed to metrics
// for the given server ID and still has the required permission.
func (c *connection) shouldReceiveMetrics(serverID string) bool {
	c.RLock()
	_, subscribed := c.subscribedMetricsServerIDs[serverID]
	c.RUnlock()
	if !subscribed {
		return false
	}

	return c.canSubscribeToServerMetrics(serverID)
}

// hasGameServerAccess returns true if the connection has access to the given game server ID.
// Superusers have access to all game servers.
func (c *connection) hasGameServerAccess(serverID string) bool {
	// currentlySuperUser() must run before taking the slice RLock because it may
	// refresh cached state under the connection write lock.
	if c.currentlySuperUser() {
		return true
	}

	c.RLock()
	defer c.RUnlock()
	return slices.Contains(c.allGameServerIDs, serverID)
}

func (c *connection) canSubscribeToServerMetrics(serverID string) bool {
	if !c.hasGameServerAccess(serverID) {
		return false
	}

	c.RLock()
	permissionLookup := c.metricsPermissionLookup
	c.RUnlock()
	if permissionLookup == nil {
		return false
	}

	allowed, errPermission := permissionLookup(serverID)
	if errPermission != nil {
		log.Warn().Err(errPermission).Str("server_id", serverID).
			Msg("Failed to authorize server metrics access")
		return false
	}
	return allowed
}

func (c *connection) hasConsolePermission(serverID string) bool {
	c.RLock()
	permissionLookup := c.consolePermissionLookup
	c.RUnlock()
	if permissionLookup == nil {
		return false
	}

	allowed, errPermission := permissionLookup(serverID)
	if errPermission != nil {
		log.Warn().Err(errPermission).Str("server_id", serverID).
			Msg("Failed to authorize game server console access")
		return false
	}
	return allowed
}

func (c *connection) removeConsoleSubscription(serverID string) {
	c.Lock()
	delete(c.requestedGameServerOutputIDs, serverID)
	c.Unlock()
}

// addGameServerAccess adds a server ID to the connection's accessible server set.
func (c *connection) addGameServerAccess(serverID string) {
	c.Lock()
	defer c.Unlock()
	if !slices.Contains(c.allGameServerIDs, serverID) {
		c.allGameServerIDs = append(c.allGameServerIDs, serverID)
	}
}

// removeGameServerAccess removes a server ID from the connection's accessible server set.
func (c *connection) removeGameServerAccess(serverID string) {
	c.Lock()
	defer c.Unlock()
	c.allGameServerIDs = slices.DeleteFunc(c.allGameServerIDs, func(id string) bool {
		return id == serverID
	})
}

func (c *connection) currentlySuperUser() bool {
	c.RLock()
	isSuperUser := c.isSuperUser
	userLookup := c.userLookup
	userID := c.userID
	lastCheck := c.lastSuperUserCheck
	c.RUnlock()

	if !isSuperUser {
		return false
	}
	if userLookup == nil || userID == "" {
		return true
	}
	if time.Since(lastCheck) < 5*time.Second {
		return true
	}

	user, errLookup := userLookup(userID)
	if errLookup != nil {
		log.Warn().Err(errLookup).Str("user_id", userID).Msg("Failed to refresh websocket superuser status")
		c.Lock()
		c.isSuperUser = false
		c.lastSuperUserCheck = time.Now()
		c.Unlock()
		return false
	}

	c.Lock()
	c.isSuperUser = user.SuperUser
	c.lastSuperUserCheck = time.Now()
	c.Unlock()
	return user.SuperUser
}

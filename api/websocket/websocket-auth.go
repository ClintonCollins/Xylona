package websocket

import (
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/olahol/melody"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/api/federation"
)

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

// shouldReceiveMetrics returns true if the connection is subscribed to metrics for the given server ID.
func (c *connection) shouldReceiveMetrics(serverID string) bool {
	c.RLock()
	defer c.RUnlock()
	_, ok := c.subscribedMetricsServerIDs[serverID]
	return ok
}

// hasGameServerAccess returns true if the connection has access to the given game server ID.
// Superusers have access to all game servers.
func (c *connection) hasGameServerAccess(serverID string) bool {
	if c.currentlySuperUser() {
		return true
	}

	c.RLock()
	defer c.RUnlock()
	return slices.Contains(c.allGameServerIDs, serverID)
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

func (ws *WebSocket) applyFederatedActingIdentity(header http.Header, userID string) error {
	errApply := federation.ApplyActingIdentityHeadersForUserID(ws.db, header, userID)
	if errApply != nil {
		return fmt.Errorf("websocket: apply federated acting identity: %w", errApply)
	}
	return nil
}

// Package federation provides helpers for cross-node federation requests.
package federation

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ClintonCollins/Xylona/db"
	fedhelpers "github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// ApplyActingIdentityHeadersForUser sets federation acting-identity headers for a known user.
// A nil user is an intentional no-op.
func ApplyActingIdentityHeadersForUser(dbConn *db.Connection, header http.Header, actingUser *models.User) error {
	if actingUser == nil {
		return nil
	}

	localSettings, errSettings := dbConn.GetLocalSettings()
	if errSettings != nil {
		return fmt.Errorf("load local settings for acting identity: %w", errSettings)
	}

	fedhelpers.ApplyActingIdentityHeaders(header, actingUser, localSettings.NodeID)
	return nil
}

// ApplyActingIdentityHeadersForUserID resolves the user and then applies federation acting-identity headers.
func ApplyActingIdentityHeadersForUserID(dbConn *db.Connection, header http.Header, userID string) error {
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return nil
	}

	user, errGetUser := dbConn.GetUserByID(trimmedUserID)
	if errGetUser != nil {
		return fmt.Errorf("load acting user %s: %w", trimmedUserID, errGetUser)
	}

	return ApplyActingIdentityHeadersForUser(dbConn, header, user)
}

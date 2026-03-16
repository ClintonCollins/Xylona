package federation

import (
	"net/http"
	"strings"

	"github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/helpers"
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
		return errSettings
	}

	helpers.ApplyFederatedActingIdentityHeaders(header, actingUser, localSettings.NodeID)
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
		return errGetUser
	}

	return ApplyActingIdentityHeadersForUser(dbConn, header, user)
}

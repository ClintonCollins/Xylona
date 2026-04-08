// Package federation provides federation mTLS identity, HTTP clients, and
// acting-identity header management for cross-node federation requests.
package federation

import (
	"net/http"
	"strings"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// ActingUserIDHeader identifies the proxied acting user in federation requests.
	ActingUserIDHeader = "X-Xylona-Acting-User-ID"
	// OriginNodeIDHeader identifies the source node for a proxied federation request.
	OriginNodeIDHeader = "X-Xylona-Origin-Node-ID"
	// ActingSuperHeader marks a proxied federation request as coming from a superuser.
	ActingSuperHeader = "X-Xylona-Acting-Super-User"
)

// ApplyActingIdentityHeaders applies acting-user identity headers for a proxied federation request.
// Passing a nil acting user intentionally leaves headers unchanged.
func ApplyActingIdentityHeaders(header http.Header, actingUser *models.User, localNodeID string) {
	if actingUser == nil {
		return
	}

	header.Set(ActingUserIDHeader, actingUser.ID)
	header.Set(OriginNodeIDHeader, localNodeID)
	if actingUser.SuperUser {
		header.Set(ActingSuperHeader, "true")
	} else {
		header.Del(ActingSuperHeader)
	}
}

// GetActingIdentity returns the acting user ID and origin node ID from federation headers.
func GetActingIdentity(header http.Header) (string, string) {
	return header.Get(ActingUserIDHeader), header.Get(OriginNodeIDHeader)
}

// ActingIsSuperUser reports whether the federated acting identity is marked as a superuser.
func ActingIsSuperUser(header http.Header) bool {
	return strings.EqualFold(strings.TrimSpace(header.Get(ActingSuperHeader)), "true")
}

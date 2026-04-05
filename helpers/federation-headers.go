package helpers

import (
	"net/http"
	"strings"

	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// FederationActingUserIDHeader identifies the proxied acting user in federation requests.
	FederationActingUserIDHeader = "X-Xylona-Acting-User-ID"
	// FederationOriginNodeIDHeader identifies the source node for a proxied federation request.
	FederationOriginNodeIDHeader = "X-Xylona-Origin-Node-ID"
	// FederationActingSuperHeader marks a proxied federation request as coming from a superuser.
	FederationActingSuperHeader = "X-Xylona-Acting-Super-User"
)

// ApplyFederatedActingIdentityHeaders applies acting-user identity headers for a proxied federation request.
// Passing a nil acting user intentionally leaves headers unchanged.
func ApplyFederatedActingIdentityHeaders(header http.Header, actingUser *models.User, localNodeID string) {
	if actingUser == nil {
		return
	}

	header.Set(FederationActingUserIDHeader, actingUser.ID)
	header.Set(FederationOriginNodeIDHeader, localNodeID)
	if actingUser.SuperUser {
		header.Set(FederationActingSuperHeader, "true")
	} else {
		header.Del(FederationActingSuperHeader)
	}
}

// GetFederatedActingIdentity returns the acting user ID and origin node ID from federation headers.
func GetFederatedActingIdentity(header http.Header) (string, string) {
	return header.Get(FederationActingUserIDHeader), header.Get(FederationOriginNodeIDHeader)
}

// FederatedActingIsSuperUser reports whether the federated acting identity is marked as a superuser.
func FederatedActingIsSuperUser(header http.Header) bool {
	return strings.EqualFold(strings.TrimSpace(header.Get(FederationActingSuperHeader)), "true")
}

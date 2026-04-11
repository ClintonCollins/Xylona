// Package federation provides helpers for cross-node federation requests.
package federation

import (
	"context"
	"strings"
)

// PeerIdentityContextKey identifies context values that store a federation peer identity.
type PeerIdentityContextKey string

// PeerIdentityKey is the context key used to store federation peer identity.
const PeerIdentityKey PeerIdentityContextKey = "federation-peer-identity"

// PeerIdentity describes the authenticated remote peer on a federation request.
type PeerIdentity struct {
	NodeID      string
	PeerNodeID  string
	Fingerprint string
}

// WithPeerIdentity stores an authenticated federation peer identity in context.
func WithPeerIdentity(ctx context.Context, identity PeerIdentity) context.Context {
	return context.WithValue(ctx, PeerIdentityKey, identity)
}

// PeerIdentityFromContext returns the authenticated peer identity stored in context.
func PeerIdentityFromContext(ctx context.Context) (PeerIdentity, bool) {
	value := ctx.Value(PeerIdentityKey)
	if value == nil {
		return PeerIdentity{}, false
	}
	identity, ok := value.(PeerIdentity)
	if !ok {
		return PeerIdentity{}, false
	}
	if strings.TrimSpace(identity.NodeID) == "" {
		return PeerIdentity{}, false
	}
	return identity, true
}

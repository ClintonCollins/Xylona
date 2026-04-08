package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers/federation"
)

// FederationLocalIdentity stores the local node's federation identity material.
type FederationLocalIdentity struct {
	NodeID          string
	CertPEM         string
	KeyPEM          string
	CertFingerprint string
}

// FederationTrustedPeer stores trust metadata for a remote federation peer.
type FederationTrustedPeer struct {
	NodeID          string
	PeerNodeID      string
	PeerFingerprint string
	Enabled         bool
	Revoked         bool
}

// UpsertFederationLocalIdentity stores the local federation identity.
func (c *Connection) UpsertFederationLocalIdentity(nodeID string, certPEM string, keyPEM string, certFingerprint string) error {
	_, errExec := sqlite.RawQuery(
		`insert into federation_local_identity
			(id, node_id, cert_path, key_path, cert_pem, key_pem, cert_fingerprint, updated_at)
		 values (1, ?, '', '', ?, ?, ?, current_timestamp)
		 on conflict(id) do update set
			node_id = excluded.node_id,
			cert_path = '',
			key_path = '',
			cert_pem = excluded.cert_pem,
			key_pem = excluded.key_pem,
			cert_fingerprint = excluded.cert_fingerprint,
			updated_at = current_timestamp`,
		nodeID,
		certPEM,
		keyPEM,
		certFingerprint,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("upsert federation local identity: %w", errExec)
	}
	return nil
}

// GetFederationLocalIdentity returns the stored local federation identity.
func (c *Connection) GetFederationLocalIdentity() (*FederationLocalIdentity, error) {
	identity := &FederationLocalIdentity{}
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`select node_id, cert_pem, key_pem, cert_fingerprint
		 from federation_local_identity
		 where id = 1`,
	).Scan(
		&identity.NodeID,
		&identity.CertPEM,
		&identity.KeyPEM,
		&identity.CertFingerprint,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("get federation local identity: %w", errQuery)
	}
	return identity, nil
}

// UpsertFederationTrustedPeer stores or updates trust metadata for a peer.
func (c *Connection) UpsertFederationTrustedPeer(nodeID string, peerNodeID string, peerFingerprint string, enabled bool, revoked bool) error {
	_, errExec := sqlite.RawQuery(
		`insert into federation_trusted_peer
			(node_id, peer_node_id, peer_fingerprint, enabled, revoked, updated_at)
		 values (?, ?, ?, ?, ?, current_timestamp)
		 on conflict(node_id) do update set
			peer_node_id = excluded.peer_node_id,
			peer_fingerprint = excluded.peer_fingerprint,
			enabled = excluded.enabled,
			revoked = excluded.revoked,
			updated_at = current_timestamp`,
		nodeID,
		peerNodeID,
		peerFingerprint,
		enabled,
		revoked,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("upsert federation trusted peer: %w", errExec)
	}
	return nil
}

// GetFederationTrustedPeerByNodeID returns a trusted peer by node ID.
func (c *Connection) GetFederationTrustedPeerByNodeID(nodeID string) (*FederationTrustedPeer, error) {
	return c.getFederationTrustedPeer(
		`select node_id, peer_node_id, peer_fingerprint, enabled, revoked
		 from federation_trusted_peer
		 where node_id = ?`,
		nodeID,
	)
}

// GetFederationTrustedPeerByFingerprint returns a trusted peer by fingerprint.
func (c *Connection) GetFederationTrustedPeerByFingerprint(peerFingerprint string) (*FederationTrustedPeer, error) {
	return c.getFederationTrustedPeer(
		`select node_id, peer_node_id, peer_fingerprint, enabled, revoked
		 from federation_trusted_peer
		 where peer_fingerprint = ?`,
		peerFingerprint,
	)
}

// RevokeFederationTrustedPeer updates a peer's revoked state.
func (c *Connection) RevokeFederationTrustedPeer(nodeID string, revoked bool) error {
	_, errExec := sqlite.RawQuery(
		`update federation_trusted_peer
		 set revoked = ?, updated_at = current_timestamp
		 where node_id = ?`,
		revoked,
		nodeID,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("revoke federation trusted peer: %w", errExec)
	}
	return nil
}

// SetFederationTrustedPeerEnabled updates whether a peer is enabled.
func (c *Connection) SetFederationTrustedPeerEnabled(nodeID string, enabled bool) error {
	_, errExec := sqlite.RawQuery(
		`update federation_trusted_peer
		 set enabled = ?, updated_at = current_timestamp
		 where node_id = ?`,
		enabled,
		nodeID,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("set federation trusted peer enabled: %w", errExec)
	}
	return nil
}

// GetFederationTrustedPeerLookup returns the trust info needed for outbound mTLS.
func (c *Connection) GetFederationTrustedPeerLookup(nodeID string) (*federation.TrustedPeerInfo, error) {
	peer, errPeer := c.GetFederationTrustedPeerByNodeID(nodeID)
	if errPeer != nil {
		return nil, errPeer
	}
	return &federation.TrustedPeerInfo{
		PeerNodeID:      peer.PeerNodeID,
		PeerFingerprint: peer.PeerFingerprint,
		Enabled:         peer.Enabled,
		Revoked:         peer.Revoked,
	}, nil
}

func (c *Connection) getFederationTrustedPeer(query string, arg string) (*FederationTrustedPeer, error) {
	peer := &FederationTrustedPeer{}
	errQuery := c.SQLDb.QueryRowContext(c.ctx, query, arg).Scan(
		&peer.NodeID,
		&peer.PeerNodeID,
		&peer.PeerFingerprint,
		&peer.Enabled,
		&peer.Revoked,
	)
	if errQuery != nil {
		if errors.Is(errQuery, sql.ErrNoRows) {
			return nil, fmt.Errorf("get federation trusted peer: %w", sql.ErrNoRows)
		}
		return nil, fmt.Errorf("get federation trusted peer: %w", errQuery)
	}
	return peer, nil
}

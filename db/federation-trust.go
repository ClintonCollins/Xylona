package db

import (
	"database/sql"
	"errors"

	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers"
)

type FederationLocalIdentity struct {
	NodeID          string
	CertPEM         string
	KeyPEM          string
	CertFingerprint string
}

type FederationTrustedPeer struct {
	NodeID          string
	PeerNodeID      string
	PeerFingerprint string
	Enabled         bool
	Revoked         bool
}

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
	return errExec
}

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
		return nil, errQuery
	}
	return identity, nil
}

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
	return errExec
}

func (c *Connection) GetFederationTrustedPeerByNodeID(nodeID string) (*FederationTrustedPeer, error) {
	return c.getFederationTrustedPeer(
		`select node_id, peer_node_id, peer_fingerprint, enabled, revoked
		 from federation_trusted_peer
		 where node_id = ?`,
		nodeID,
	)
}

func (c *Connection) GetFederationTrustedPeerByFingerprint(peerFingerprint string) (*FederationTrustedPeer, error) {
	return c.getFederationTrustedPeer(
		`select node_id, peer_node_id, peer_fingerprint, enabled, revoked
		 from federation_trusted_peer
		 where peer_fingerprint = ?`,
		peerFingerprint,
	)
}

func (c *Connection) RevokeFederationTrustedPeer(nodeID string, revoked bool) error {
	_, errExec := sqlite.RawQuery(
		`update federation_trusted_peer
		 set revoked = ?, updated_at = current_timestamp
		 where node_id = ?`,
		revoked,
		nodeID,
	).Exec(c.ctx, c.DB)
	return errExec
}

func (c *Connection) SetFederationTrustedPeerEnabled(nodeID string, enabled bool) error {
	_, errExec := sqlite.RawQuery(
		`update federation_trusted_peer
		 set enabled = ?, updated_at = current_timestamp
		 where node_id = ?`,
		enabled,
		nodeID,
	).Exec(c.ctx, c.DB)
	return errExec
}

// GetFederationTrustedPeerLookup returns the trusted peer info in the format needed by helpers.TrustedPeerLookup.
func (c *Connection) GetFederationTrustedPeerLookup(nodeID string) (*helpers.TrustedPeerInfo, error) {
	peer, errPeer := c.GetFederationTrustedPeerByNodeID(nodeID)
	if errPeer != nil {
		return nil, errPeer
	}
	return &helpers.TrustedPeerInfo{
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
			return nil, sql.ErrNoRows
		}
		return nil, errQuery
	}
	return peer, nil
}

package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/stephenafamo/bob/dialect/sqlite"

	"github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

const (
	federationLocalIdentityKeyPEMFormatPlaintext   = "plaintext"
	federationLocalIdentityKeyPEMFormatEncryptedV1 = "aes256gcm-v1"
)

// FederationLocalIdentity stores the local node's federation identity material.
type FederationLocalIdentity struct {
	NodeID          string
	CertPEM         string
	KeyPEM          string
	KeyPEMFormat    string
	CertFingerprint string
}

// KeyPEMStoredEncrypted reports whether the stored federation private key is
// using the encrypted at-rest format.
func (f *FederationLocalIdentity) KeyPEMStoredEncrypted() bool {
	return f.KeyPEMFormat == federationLocalIdentityKeyPEMFormatEncryptedV1
}

// FederationTrustedPeer stores trust metadata for a remote federation peer.
type FederationTrustedPeer struct {
	NodeID          string
	PeerNodeID      string
	PeerFingerprint string
	Enabled         bool
	Revoked         bool
}

type federationLocalIdentityRow struct {
	NodeID          string
	CertPEM         string
	KeyPEM          string
	KeyPEMFormat    string
	CertFingerprint string
}

// UpsertFederationLocalIdentity stores the local federation identity.
func (c *Connection) UpsertFederationLocalIdentity(nodeID string, certPEM string, keyPEM string, certFingerprint string) error {
	encryptedKeyPEM, errEncryptKeyPEM := c.encryptFederationLocalIdentityKeyPEM(keyPEM)
	if errEncryptKeyPEM != nil {
		return fmt.Errorf("upsert federation local identity: %w", errEncryptKeyPEM)
	}

	_, errExec := sqlite.RawQuery(
		`insert into federation_local_identity
			(id, node_id, cert_path, key_path, cert_pem, key_pem, key_pem_format, cert_fingerprint, updated_at)
		 values (1, ?, '', '', ?, ?, ?, ?, current_timestamp)
		 on conflict(id) do update set
			node_id = excluded.node_id,
			cert_path = '',
			key_path = '',
			cert_pem = excluded.cert_pem,
			key_pem = excluded.key_pem,
			key_pem_format = excluded.key_pem_format,
			cert_fingerprint = excluded.cert_fingerprint,
			updated_at = current_timestamp`,
		nodeID,
		certPEM,
		encryptedKeyPEM,
		federationLocalIdentityKeyPEMFormatEncryptedV1,
		certFingerprint,
	).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("upsert federation local identity: %w", errExec)
	}
	return nil
}

// GetFederationLocalIdentity returns the stored local federation identity.
func (c *Connection) GetFederationLocalIdentity() (*FederationLocalIdentity, error) {
	row, errGetRow := c.getFederationLocalIdentityRow()
	if errGetRow != nil {
		return nil, fmt.Errorf("get federation local identity: %w", errGetRow)
	}

	keyPEM, errDecryptKeyPEM := c.decryptFederationLocalIdentityKeyPEM(row.KeyPEM, row.KeyPEMFormat)
	if errDecryptKeyPEM != nil {
		return nil, fmt.Errorf("get federation local identity: %w", errDecryptKeyPEM)
	}

	return &FederationLocalIdentity{
		NodeID:          row.NodeID,
		CertPEM:         row.CertPEM,
		KeyPEM:          keyPEM,
		KeyPEMFormat:    row.KeyPEMFormat,
		CertFingerprint: row.CertFingerprint,
	}, nil
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

func (c *Connection) getFederationLocalIdentityRow() (*federationLocalIdentityRow, error) {
	row := &federationLocalIdentityRow{}
	errQuery := c.SQLDb.QueryRowContext(
		c.ctx,
		`select node_id, cert_pem, key_pem, key_pem_format, cert_fingerprint
		 from federation_local_identity
		 where id = 1`,
	).Scan(
		&row.NodeID,
		&row.CertPEM,
		&row.KeyPEM,
		&row.KeyPEMFormat,
		&row.CertFingerprint,
	)
	if errQuery != nil {
		return nil, fmt.Errorf("query federation local identity row: %w", errQuery)
	}
	return row, nil
}

func (c *Connection) encryptFederationLocalIdentityKeyPEM(keyPEM string) (string, error) {
	if keyPEM == "" {
		return "", nil
	}
	if len(c.encryptionKey) != xycrypt.EncryptionKeySize {
		return "", errors.New("encrypt federation local identity key PEM: encryption key is not configured")
	}

	encryptedKeyPEM, errEncrypt := xycrypt.Encrypt(c.encryptionKey, keyPEM)
	if errEncrypt != nil {
		return "", fmt.Errorf("encrypt federation local identity key PEM: %w", errEncrypt)
	}
	return encryptedKeyPEM, nil
}

func (c *Connection) decryptFederationLocalIdentityKeyPEM(storedKeyPEM string, format string) (string, error) {
	if storedKeyPEM == "" {
		return "", nil
	}

	switch format {
	case "", federationLocalIdentityKeyPEMFormatPlaintext:
		return storedKeyPEM, nil
	case federationLocalIdentityKeyPEMFormatEncryptedV1:
		if len(c.encryptionKey) != xycrypt.EncryptionKeySize {
			return "", errors.New("decrypt federation local identity key PEM: encryption key is not configured")
		}

		decryptedKeyPEM, errDecrypt := xycrypt.Decrypt(c.encryptionKey, storedKeyPEM)
		if errDecrypt != nil {
			return "", fmt.Errorf("decrypt federation local identity key PEM: %w", errDecrypt)
		}
		return decryptedKeyPEM, nil
	default:
		return "", fmt.Errorf("decrypt federation local identity key PEM: unsupported format %q", format)
	}
}

// MigrateLegacyFederationLocalIdentityKeyPEM encrypts any legacy plaintext
// federation private key already stored in the database.
func (c *Connection) MigrateLegacyFederationLocalIdentityKeyPEM() error {
	row, errGetRow := c.getFederationLocalIdentityRow()
	if errGetRow != nil {
		if errors.Is(errGetRow, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("migrate federation local identity key PEM: %w", errGetRow)
	}

	if row.KeyPEM == "" {
		return nil
	}
	if row.KeyPEMFormat == federationLocalIdentityKeyPEMFormatEncryptedV1 {
		return nil
	}
	if row.KeyPEMFormat != "" && row.KeyPEMFormat != federationLocalIdentityKeyPEMFormatPlaintext {
		return fmt.Errorf("migrate federation local identity key PEM: unsupported format %q", row.KeyPEMFormat)
	}

	encryptedKeyPEM, errEncryptKeyPEM := c.encryptFederationLocalIdentityKeyPEM(row.KeyPEM)
	if errEncryptKeyPEM != nil {
		return fmt.Errorf("migrate federation local identity key PEM: %w", errEncryptKeyPEM)
	}

	tx, errBeginTx := c.SQLDb.BeginTx(c.ctx, nil)
	if errBeginTx != nil {
		return fmt.Errorf("migrate federation local identity key PEM: begin transaction: %w", errBeginTx)
	}

	_, errExec := tx.ExecContext(
		c.ctx,
		`update federation_local_identity
		 set key_pem = ?, key_pem_format = ?, updated_at = current_timestamp
		 where id = 1`,
		encryptedKeyPEM,
		federationLocalIdentityKeyPEMFormatEncryptedV1,
	)
	if errExec != nil {
		errRollback := tx.Rollback()
		if errRollback != nil {
			return fmt.Errorf("migrate federation local identity key PEM: update row: %w; rollback: %s", errExec, errRollback.Error())
		}
		return fmt.Errorf("migrate federation local identity key PEM: update row: %w", errExec)
	}

	errCommit := tx.Commit()
	if errCommit != nil {
		return fmt.Errorf("migrate federation local identity key PEM: commit transaction: %w", errCommit)
	}

	return nil
}

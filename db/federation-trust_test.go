package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestFederationLocalIdentityCRUD(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federation-local-identity.sqlite")

	errUpsert := conn.UpsertFederationLocalIdentity(
		"local-node-1",
		"cert-pem-1",
		"key-pem-1",
		"fp-local-1",
	)
	if errUpsert != nil {
		t.Fatalf("UpsertFederationLocalIdentity() error = %v", errUpsert)
	}

	gotIdentity, errGet := conn.GetFederationLocalIdentity()
	if errGet != nil {
		t.Fatalf("GetFederationLocalIdentity() error = %v", errGet)
	}

	if gotIdentity.NodeID != "local-node-1" {
		t.Errorf("NodeID = %q, want %q", gotIdentity.NodeID, "local-node-1")
	}
	if gotIdentity.CertPEM != "cert-pem-1" {
		t.Errorf("CertPEM = %q, want %q", gotIdentity.CertPEM, "cert-pem-1")
	}
	if gotIdentity.KeyPEM != "key-pem-1" {
		t.Errorf("KeyPEM = %q, want %q", gotIdentity.KeyPEM, "key-pem-1")
	}
	if gotIdentity.CertFingerprint != "fp-local-1" {
		t.Errorf("CertFingerprint = %q, want %q", gotIdentity.CertFingerprint, "fp-local-1")
	}

	errUpsert = conn.UpsertFederationLocalIdentity(
		"local-node-2",
		"cert-pem-2",
		"key-pem-2",
		"fp-local-2",
	)
	if errUpsert != nil {
		t.Fatalf("UpsertFederationLocalIdentity() second call error = %v", errUpsert)
	}

	gotIdentity, errGet = conn.GetFederationLocalIdentity()
	if errGet != nil {
		t.Fatalf("GetFederationLocalIdentity() after update error = %v", errGet)
	}

	if gotIdentity.NodeID != "local-node-2" {
		t.Errorf("NodeID = %q, want %q", gotIdentity.NodeID, "local-node-2")
	}
	if gotIdentity.CertFingerprint != "fp-local-2" {
		t.Errorf("CertFingerprint = %q, want %q", gotIdentity.CertFingerprint, "fp-local-2")
	}
	if gotIdentity.CertPEM != "cert-pem-2" {
		t.Errorf("CertPEM = %q, want %q", gotIdentity.CertPEM, "cert-pem-2")
	}
	if gotIdentity.KeyPEM != "key-pem-2" {
		t.Errorf("KeyPEM = %q, want %q", gotIdentity.KeyPEM, "key-pem-2")
	}
}

func TestFederationTrustedPeerCRUD(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federation-trusted-peer.sqlite")

	_, errInsertNode := conn.SQLDb.ExecContext(conn.ctx, `
		INSERT INTO node (id, name, host, port) VALUES ('remote-node-row-1', 'Remote Node', '', 0)
	`)
	if errInsertNode != nil {
		t.Fatalf("failed to insert node row: %v", errInsertNode)
	}

	errUpsert := conn.UpsertFederationTrustedPeer(
		"remote-node-row-1",
		"remote-node-id-1",
		"fp-remote-1",
		true,
		false,
	)
	if errUpsert != nil {
		t.Fatalf("UpsertFederationTrustedPeer() error = %v", errUpsert)
	}

	peerByNodeID, errByNode := conn.GetFederationTrustedPeerByNodeID("remote-node-row-1")
	if errByNode != nil {
		t.Fatalf("GetFederationTrustedPeerByNodeID() error = %v", errByNode)
	}

	if peerByNodeID.PeerNodeID != "remote-node-id-1" {
		t.Errorf("PeerNodeID = %q, want %q", peerByNodeID.PeerNodeID, "remote-node-id-1")
	}
	if peerByNodeID.PeerFingerprint != "fp-remote-1" {
		t.Errorf("PeerFingerprint = %q, want %q", peerByNodeID.PeerFingerprint, "fp-remote-1")
	}
	if !peerByNodeID.Enabled {
		t.Errorf("Enabled = %v, want %v", peerByNodeID.Enabled, true)
	}
	if peerByNodeID.Revoked {
		t.Errorf("Revoked = %v, want %v", peerByNodeID.Revoked, false)
	}

	peerByFingerprint, errByFingerprint := conn.GetFederationTrustedPeerByFingerprint("fp-remote-1")
	if errByFingerprint != nil {
		t.Fatalf("GetFederationTrustedPeerByFingerprint() error = %v", errByFingerprint)
	}
	if peerByFingerprint.NodeID != "remote-node-row-1" {
		t.Errorf("NodeID = %q, want %q", peerByFingerprint.NodeID, "remote-node-row-1")
	}

	errUpsert = conn.UpsertFederationTrustedPeer(
		"remote-node-row-1",
		"remote-node-id-1",
		"fp-remote-2",
		false,
		true,
	)
	if errUpsert != nil {
		t.Fatalf("UpsertFederationTrustedPeer() update error = %v", errUpsert)
	}

	peerByNodeID, errByNode = conn.GetFederationTrustedPeerByNodeID("remote-node-row-1")
	if errByNode != nil {
		t.Fatalf("GetFederationTrustedPeerByNodeID() after update error = %v", errByNode)
	}
	if peerByNodeID.PeerFingerprint != "fp-remote-2" {
		t.Errorf("PeerFingerprint = %q, want %q", peerByNodeID.PeerFingerprint, "fp-remote-2")
	}
	if peerByNodeID.Enabled {
		t.Errorf("Enabled = %v, want %v", peerByNodeID.Enabled, false)
	}
	if !peerByNodeID.Revoked {
		t.Errorf("Revoked = %v, want %v", peerByNodeID.Revoked, true)
	}

	_, errMissing := conn.GetFederationTrustedPeerByFingerprint("missing")
	if errMissing == nil {
		t.Fatalf("GetFederationTrustedPeerByFingerprint() error = nil, want sql.ErrNoRows")
	}
	if !errors.Is(errMissing, sql.ErrNoRows) {
		t.Fatalf("GetFederationTrustedPeerByFingerprint() error = %v, want %v", errMissing, sql.ErrNoRows)
	}
}

func TestFederationTrustedPeerRevokeAndEnable(t *testing.T) {
	conn := newRBACMigratedConnection(t, "federation-revoke-enable.sqlite")

	_, errInsertNode := conn.SQLDb.ExecContext(conn.ctx, `
		INSERT INTO node (id, name, host, port) VALUES ('node-1', 'Test Node', '', 0)
	`)
	if errInsertNode != nil {
		t.Fatalf("failed to insert node row: %v", errInsertNode)
	}

	errUpsert := conn.UpsertFederationTrustedPeer("node-1", "peer-1", "fp-1", true, false)
	if errUpsert != nil {
		t.Fatalf("UpsertFederationTrustedPeer() error = %v", errUpsert)
	}

	// Test RevokeFederationTrustedPeer
	errRevoke := conn.RevokeFederationTrustedPeer("node-1", true)
	if errRevoke != nil {
		t.Fatalf("RevokeFederationTrustedPeer() error = %v", errRevoke)
	}
	peer, errGet := conn.GetFederationTrustedPeerByNodeID("node-1")
	if errGet != nil {
		t.Fatalf("GetFederationTrustedPeerByNodeID() error = %v", errGet)
	}
	if !peer.Revoked {
		t.Errorf("Revoked = %v, want true", peer.Revoked)
	}

	// Unrevoke
	errUnrevoke := conn.RevokeFederationTrustedPeer("node-1", false)
	if errUnrevoke != nil {
		t.Fatalf("RevokeFederationTrustedPeer(false) error = %v", errUnrevoke)
	}
	peer, _ = conn.GetFederationTrustedPeerByNodeID("node-1")
	if peer.Revoked {
		t.Errorf("Revoked = %v, want false", peer.Revoked)
	}

	// Test SetFederationTrustedPeerEnabled
	errDisable := conn.SetFederationTrustedPeerEnabled("node-1", false)
	if errDisable != nil {
		t.Fatalf("SetFederationTrustedPeerEnabled(false) error = %v", errDisable)
	}
	peer, _ = conn.GetFederationTrustedPeerByNodeID("node-1")
	if peer.Enabled {
		t.Errorf("Enabled = %v, want false", peer.Enabled)
	}

	errEnable := conn.SetFederationTrustedPeerEnabled("node-1", true)
	if errEnable != nil {
		t.Fatalf("SetFederationTrustedPeerEnabled(true) error = %v", errEnable)
	}
	peer, _ = conn.GetFederationTrustedPeerByNodeID("node-1")
	if !peer.Enabled {
		t.Errorf("Enabled = %v, want true", peer.Enabled)
	}

	// Test GetFederationTrustedPeerLookup
	lookupInfo, errLookup := conn.GetFederationTrustedPeerLookup("node-1")
	if errLookup != nil {
		t.Fatalf("GetFederationTrustedPeerLookup() error = %v", errLookup)
	}
	if lookupInfo.PeerNodeID != "peer-1" {
		t.Errorf("PeerNodeID = %q, want %q", lookupInfo.PeerNodeID, "peer-1")
	}
	if lookupInfo.PeerFingerprint != "fp-1" {
		t.Errorf("PeerFingerprint = %q, want %q", lookupInfo.PeerFingerprint, "fp-1")
	}
	if !lookupInfo.Enabled {
		t.Errorf("Enabled = %v, want true", lookupInfo.Enabled)
	}
	if lookupInfo.Revoked {
		t.Errorf("Revoked = %v, want false", lookupInfo.Revoked)
	}

	// Missing node returns error
	_, errMissing := conn.GetFederationTrustedPeerLookup("nonexistent")
	if errMissing == nil {
		t.Fatalf("GetFederationTrustedPeerLookup(nonexistent) error = nil, want error")
	}
}

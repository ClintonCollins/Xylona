package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	dbpkg "github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
)

// dialRegisteredRemoteNodes enumerates every enabled node in the DB other
// than the controller's embedded self-node, decrypts its shared secret, and
// registers a gRPC client in the noderegistry so the controller can route
// RPCs through it immediately.
//
// This is a best-effort operation: a missing remote node must not block
// controller startup. Individual dial failures return on the error list but
// the registry is populated with whatever succeeded.
func dialRegisteredRemoteNodes(ctx context.Context, dbInst *dbpkg.Connection, selfNodeID string, registry *noderegistry.Registry) error {
	_ = ctx // reserved; future per-node healthcheck loops will use it.

	nodes, errList := dbInst.GetAllNodes()
	if errList != nil {
		return fmt.Errorf("list nodes for dial: %w", errList)
	}

	var dialErrors []error
	for _, node := range nodes {
		if node.ID == selfNodeID {
			continue
		}
		if !node.Enabled {
			log.Debug().Str("node_id", node.ID).Msg("Skipping disabled remote node at startup")
			continue
		}
		if strings.TrimSpace(node.ListenURL) == "" || strings.TrimSpace(node.CertFingerprint) == "" {
			log.Warn().Str("node_id", node.ID).Msg("Remote node is missing listen URL or cert fingerprint; skipping dial")
			continue
		}
		if strings.TrimSpace(node.SharedSecretEncrypted) == "" {
			log.Warn().Str("node_id", node.ID).Msg("Remote node has no shared secret on file; skipping dial")
			continue
		}

		sharedSecret, errDecrypt := dbInst.DecryptText(node.SharedSecretEncrypted)
		if errDecrypt != nil {
			dialErrors = append(dialErrors, fmt.Errorf("decrypt shared secret for node %s: %w", node.ID, errDecrypt))
			continue
		}

		client, errClient := nodeclient.NewGRPCClient(node.ID, node.ListenURL, node.CertFingerprint, sharedSecret)
		if errClient != nil {
			dialErrors = append(dialErrors, fmt.Errorf("build gRPC client for node %s: %w", node.ID, errClient))
			continue
		}

		registry.Register(client)
		log.Info().
			Str("node_id", node.ID).
			Str("name", node.Name).
			Str("listen_url", node.ListenURL).
			Msg("Registered remote node in noderegistry")
	}

	if len(dialErrors) > 0 {
		return errors.Join(dialErrors...)
	}
	return nil
}

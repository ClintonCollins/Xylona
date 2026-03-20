package actions

import (
	"context"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// BuildLocalPeerList returns PeerInfo for all enabled remote nodes.
func (inst *Instance) BuildLocalPeerList() ([]*xylona.PeerInfo, error) {
	nodes, errNodes := inst.db.GetEnabledRemoteNodes()
	if errNodes != nil {
		return nil, errNodes
	}

	peers := make([]*xylona.PeerInfo, 0, len(nodes))
	for _, node := range nodes {
		trust, errTrust := inst.db.GetFederationTrustedPeerByNodeID(node.ID)
		if errTrust != nil {
			log.Warn().Err(errTrust).Str("node_id", node.ID).Msg("Skipping node without trust entry in peer list")
			continue
		}
		peers = append(peers, &xylona.PeerInfo{
			NodeId:          node.ID,
			Name:            node.Name,
			BaseUrl:         node.BaseURL,
			CertFingerprint: trust.PeerFingerprint,
			FederationPort:  int32(node.Port),
		})
	}
	return peers, nil
}

// ProcessReceivedPeerList checks for unknown peers and triggers auto-pairing.
func (inst *Instance) ProcessReceivedPeerList(peers []*xylona.PeerInfo, introducerNodeID string) {
	localSettings, errSettings := inst.db.GetLocalSettings()
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to get local settings for peer list processing")
		return
	}

	for _, peer := range peers {
		// Skip self.
		if peer.NodeId == localSettings.NodeID {
			continue
		}
		// Skip already-known nodes.
		_, errGet := inst.db.GetNodeByID(peer.NodeId)
		if errGet == nil {
			continue
		}
		// Unknown node — trigger auto-pairing.
		go inst.startAutoPairing(peer, introducerNodeID)
	}
}

// startAutoPairing creates a node record and trust entry for an introduced peer.
func (inst *Instance) startAutoPairing(peer *xylona.PeerInfo, introducerNodeID string) {
	// Idempotency check.
	_, errGet := inst.db.GetNodeByID(peer.NodeId)
	if errGet == nil {
		return // Already exists.
	}

	// Insert node record.
	_, errInsert := inst.db.InsertRemoteNode(&models.NodeSetter{
		ID:         omit.From(peer.NodeId),
		Name:       omit.From(peer.Name),
		IsLocal:    omit.From(false),
		Host:       omit.From(""),
		BaseURL:    omit.From(peer.BaseUrl),
		Port:       omit.From(int64(peer.FederationPort)),
		Enabled:    omit.From(true),
		AutoPaired: omit.From(true),
	})
	if errInsert != nil {
		log.Error().Err(errInsert).Str("node_id", peer.NodeId).Msg("Failed to insert auto-paired node")
		return
	}

	// Trust the fingerprint.
	errTrust := inst.db.UpsertFederationTrustedPeer(peer.NodeId, peer.NodeId, peer.CertFingerprint, true, false)
	if errTrust != nil {
		log.Error().Err(errTrust).Str("node_id", peer.NodeId).Msg("Failed to trust auto-paired node fingerprint")
		// Roll back node insert.
		_ = inst.db.DeleteNodeByID(peer.NodeId)
		return
	}

	// Log advisory.
	introducerName := introducerNodeID
	introducer, errIntroducer := inst.db.GetNodeByID(introducerNodeID)
	if errIntroducer == nil {
		introducerName = introducer.Name
	}

	_, errAdvisory := inst.db.InsertFederationAdvisory(&models.FederationAdvisorySetter{
		ID:                 omit.From(uuid.NewString()),
		Type:               omit.From("NODE_AUTO_PAIRED"),
		Title:              omit.From("Node auto-paired"),
		Message:            omit.From(peer.Name + " was auto-paired via introduction from " + introducerName),
		SourceNodeID:       omit.From(introducerNodeID),
		SourceNodeName:     omit.From(introducerName),
		SubjectNodeID:      omit.From(peer.NodeId),
		SubjectNodeName:    omit.From(peer.Name),
		SubjectNodeBaseURL: omit.From(peer.BaseUrl),
		Read:               omit.From(false),
	})
	if errAdvisory != nil {
		log.Error().Err(errAdvisory).Msg("Failed to create auto-pairing advisory")
	}

	log.Info().
		Str("node_id", peer.NodeId).
		Str("node_name", peer.Name).
		Str("introducer", introducerName).
		Msg("Auto-paired with introduced node")
}

// HandlePeerChange processes a NotifyPeerChangeRequest from a peer.
func (inst *Instance) HandlePeerChange(msg *xylona.NotifyPeerChangeRequest) {
	switch msg.ChangeType {
	case xylona.PeerChangeType_PEER_CHANGE_TYPE_NODE_JOINED:
		go inst.startAutoPairing(msg.Peer, msg.InitiatedByNodeId)

	case xylona.PeerChangeType_PEER_CHANGE_TYPE_NODE_REVOKED:
		inst.handleNodeRevoked(msg)

	case xylona.PeerChangeType_PEER_CHANGE_TYPE_NODE_DEPARTED:
		inst.HandleNodeDeparture(msg.Peer.NodeId, "")
	}
}

func (inst *Instance) handleNodeRevoked(msg *xylona.NotifyPeerChangeRequest) {
	nodeID := msg.Peer.NodeId

	// Remove sync worker if sync engine is available.
	if inst.syncEngine != nil {
		inst.syncEngine.RemovePeer(nodeID)
	}

	// Delete the node (cascades to cache, trust, sync state).
	errDelete := inst.db.DeleteNodeByID(nodeID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("node_id", nodeID).Msg("Failed to delete revoked node")
		return
	}

	// Log advisory.
	_, errAdvisory := inst.db.InsertFederationAdvisory(&models.FederationAdvisorySetter{
		ID:                 omit.From(uuid.NewString()),
		Type:               omit.From("NODE_REVOKED"),
		Title:              omit.From("Node removed from federation"),
		Message:            omit.From(msg.Peer.Name + " was removed by " + msg.InitiatedByNodeName),
		SourceNodeID:       omit.From(msg.InitiatedByNodeId),
		SourceNodeName:     omit.From(msg.InitiatedByNodeName),
		SubjectNodeID:      omit.From(nodeID),
		SubjectNodeName:    omit.From(msg.Peer.Name),
		SubjectNodeBaseURL: omit.From(msg.Peer.BaseUrl),
		Read:               omit.From(false),
	})
	if errAdvisory != nil {
		log.Error().Err(errAdvisory).Msg("Failed to create revocation advisory")
	}

	log.Info().Str("node_id", nodeID).Str("revoked_by", msg.InitiatedByNodeName).Msg("Revoked node removed from federation")
}

// HandleNodeDeparture removes a departing node and logs an advisory.
func (inst *Instance) HandleNodeDeparture(nodeID string, reason string) {
	node, errGet := inst.db.GetNodeByID(nodeID)
	nodeName := nodeID
	nodeBaseURL := ""
	if errGet == nil {
		nodeName = node.Name
		nodeBaseURL = node.BaseURL
	}

	if inst.syncEngine != nil {
		inst.syncEngine.RemovePeer(nodeID)
	}

	errDelete := inst.db.DeleteNodeByID(nodeID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("node_id", nodeID).Msg("Failed to delete departed node")
		return
	}

	message := nodeName + " left the federation"
	if reason != "" {
		message += " (" + reason + ")"
	}

	_, errAdvisory := inst.db.InsertFederationAdvisory(&models.FederationAdvisorySetter{
		ID:                 omit.From(uuid.NewString()),
		Type:               omit.From("NODE_DEPARTED"),
		Title:              omit.From("Node left the federation"),
		Message:            omit.From(message),
		SubjectNodeID:      omit.From(nodeID),
		SubjectNodeName:    omit.From(nodeName),
		SubjectNodeBaseURL: omit.From(nodeBaseURL),
		Read:               omit.From(false),
	})
	if errAdvisory != nil {
		log.Error().Err(errAdvisory).Msg("Failed to create departure advisory")
	}

	log.Info().Str("node_id", nodeID).Str("node_name", nodeName).Msg("Departed node removed from federation")
}

// LeaveFederation broadcasts departure to all peers, then cleans up local federation data.
func (inst *Instance) LeaveFederation() error {
	localSettings, errSettings := inst.db.GetLocalSettings()
	if errSettings != nil {
		return errSettings
	}

	// Set departed flag.
	errDeparted := inst.db.SetNodeDeparted(localSettings.NodeID, true)
	if errDeparted != nil {
		return errDeparted
	}

	// Broadcast NotifyDeparture to all peers (fire-and-forget).
	nodes, errNodes := inst.db.GetEnabledRemoteNodes()
	if errNodes == nil {
		msg := &xylona.NotifyDepartureRequest{
			NodeId: localSettings.NodeID,
			Reason: "voluntary departure",
		}
		var wg sync.WaitGroup
		for _, node := range nodes {
			wg.Add(1)
			go func(n *models.Node) {
				defer wg.Done()
				client, errClient := newFederationClient(inst.db, inst.federationMTLS, n, 10*time.Second)
				if errClient != nil {
					log.Warn().Err(errClient).Str("node_id", n.ID).Msg("Failed to notify peer of departure")
					return
				}
				ctx, cancel := context.WithTimeout(inst.ctx, 10*time.Second)
				defer cancel()
				_, errNotify := client.NotifyDeparture(ctx, connect.NewRequest(msg))
				if errNotify != nil {
					log.Warn().Err(errNotify).Str("node_id", n.ID).Msg("Failed to notify peer of departure")
				}
			}(node)
		}
		wg.Wait()
	}

	// Stop all sync workers and clean up.
	for _, node := range nodes {
		if inst.syncEngine != nil {
			inst.syncEngine.RemovePeer(node.ID)
		}
		_ = inst.db.DeleteNodeByID(node.ID)
	}

	return nil
}

// ExchangePeerListWithNode sends our peer list to a specific node and processes its response.
func (inst *Instance) ExchangePeerListWithNode(nodeID string) {
	node, errNode := inst.db.GetRemoteNodeByID(nodeID)
	if errNode != nil {
		log.Warn().Err(errNode).Str("node_id", nodeID).Msg("Failed to get node for peer list exchange")
		return
	}

	client, errClient := newFederationClient(inst.db, inst.federationMTLS, node, 30*time.Second)
	if errClient != nil {
		log.Warn().Err(errClient).Str("node_id", nodeID).Msg("Failed to create client for peer list exchange")
		return
	}

	localPeers, errBuild := inst.BuildLocalPeerList()
	if errBuild != nil {
		log.Error().Err(errBuild).Msg("Failed to build local peer list")
		return
	}

	localSettings, errSettings := inst.db.GetLocalSettings()
	if errSettings != nil {
		log.Error().Err(errSettings).Msg("Failed to get local settings")
		return
	}

	ctx, cancel := context.WithTimeout(inst.ctx, 30*time.Second)
	defer cancel()

	resp, errExchange := client.ExchangePeerList(ctx, connect.NewRequest(&xylona.ExchangePeerListRequest{
		SenderNodeId: localSettings.NodeID,
		Peers:        localPeers,
	}))
	if errExchange != nil {
		log.Warn().Err(errExchange).Str("node_id", nodeID).Msg("Peer list exchange failed")
		return
	}

	// Process received peers for auto-pairing.
	inst.ProcessReceivedPeerList(resp.Msg.Peers, nodeID)
	log.Info().Str("node_id", nodeID).Int("remote_peers", len(resp.Msg.Peers)).Msg("Peer list exchanged with newly paired node")
}

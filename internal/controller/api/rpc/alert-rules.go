package rpc

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// isNodeEventType returns true for event types that apply to nodes rather than
// individual game servers.
func isNodeEventType(et xylona.AlertEventType) bool {
	return et == xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_CPU_THRESHOLD ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_MEMORY_THRESHOLD ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_NODE_DISK_THRESHOLD
}

// isServerEventType returns true for event types that apply to individual game
// servers.
func isServerEventType(et xylona.AlertEventType) bool {
	return et == xylona.AlertEventType_ALERT_EVENT_TYPE_CRASH ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_STATUS_CHANGE ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_CPU_THRESHOLD ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_MEMORY_THRESHOLD ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_DISK_THRESHOLD ||
		et == xylona.AlertEventType_ALERT_EVENT_TYPE_PLAYER_COUNT_THRESHOLD
}

// alertRuleToProto converts a DB model alert rule to its protobuf
// representation.
func alertRuleToProto(rule *models.AlertRule) *xylona.AlertRule {
	eventTypeInt, ok := xylona.AlertEventType_value[rule.EventType]
	eventType := xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED
	if ok {
		eventType = xylona.AlertEventType(eventTypeInt)
	}

	proto := &xylona.AlertRule{
		Id:                    rule.ID,
		UserId:                rule.UserID,
		EventType:             eventType,
		NotificationChannelId: rule.NotificationChannelID,
		Enabled:               rule.Enabled != 0,
		CreatedAt:             timestamppb.New(rule.CreatedAt),
		UpdatedAt:             timestamppb.New(rule.UpdatedAt),
	}

	serverID, serverIDSet := rule.ServerID.Get()
	if serverIDSet {
		proto.ServerId = &serverID
	}

	serverNodeID, serverNodeIDSet := rule.ServerNodeID.Get()
	if serverNodeIDSet {
		proto.ServerNodeId = &serverNodeID
	}

	nodeID, nodeIDSet := rule.NodeID.Get()
	if nodeIDSet {
		proto.NodeId = &nodeID
	}

	condition, conditionSet := rule.Condition.Get()
	if conditionSet {
		proto.Condition = condition
	}

	return proto
}

// alertHistoryToProto converts a DB model alert history entry to its protobuf
// representation.
func alertHistoryToProto(h *models.AlertHistory) *xylona.AlertHistoryEntry {
	eventTypeInt, ok := xylona.AlertEventType_value[h.EventType]
	eventType := xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED
	if ok {
		eventType = xylona.AlertEventType(eventTypeInt)
	}

	channelTypeInt, chOk := xylona.NotificationChannelType_value[h.ChannelType]
	channelType := xylona.NotificationChannelType_NOTIFICATION_CHANNEL_TYPE_UNSPECIFIED
	if chOk {
		channelType = xylona.NotificationChannelType(channelTypeInt)
	}

	deliveryStatusInt, dsOk := xylona.DeliveryStatus_value[h.DeliveryStatus]
	deliveryStatus := xylona.DeliveryStatus_DELIVERY_STATUS_UNSPECIFIED
	if dsOk {
		deliveryStatus = xylona.DeliveryStatus(deliveryStatusInt)
	}

	proto := &xylona.AlertHistoryEntry{
		Id:             h.ID,
		UserId:         h.UserID,
		EventType:      eventType,
		ChannelType:    channelType,
		DeliveryStatus: deliveryStatus,
		CreatedAt:      timestamppb.New(h.CreatedAt),
	}

	alertRuleID, ruleIDSet := h.AlertRuleID.Get()
	if ruleIDSet {
		proto.AlertRuleId = &alertRuleID
	}

	serverID, serverIDSet := h.ServerID.Get()
	if serverIDSet {
		proto.ServerId = &serverID
	}

	serverNodeID, serverNodeIDSet := h.ServerNodeID.Get()
	if serverNodeIDSet {
		proto.ServerNodeId = &serverNodeID
	}

	nodeID, nodeIDSet := h.NodeID.Get()
	if nodeIDSet {
		proto.NodeId = &nodeID
	}

	eventData, eventDataSet := h.EventData.Get()
	if eventDataSet {
		proto.EventData = eventData
	}

	deliveryError, deliveryErrSet := h.DeliveryError.Get()
	if deliveryErrSet {
		proto.DeliveryError = &deliveryError
	}

	return proto
}

// validateAlertRuleRequest performs shared validation for create and update
// requests. It returns the extracted serverID, serverNodeID, and nodeID
// strings (empty string when unset), or an error if validation fails.
func (xs *XylonaService) validateAlertRuleRequest(
	user *models.User,
	eventType xylona.AlertEventType,
	notificationChannelID string,
	serverIDPtr, serverNodeIDPtr, nodeIDPtr *string,
) (serverID, serverNodeID, nodeID string, err error) {
	// Validate event_type != UNSPECIFIED
	if eventType == xylona.AlertEventType_ALERT_EVENT_TYPE_UNSPECIFIED {
		return "", "", "", invalidArg("event_type is required")
	}

	// Validate notification_channel_id is not empty
	if strings.TrimSpace(notificationChannelID) == "" {
		return "", "", "", invalidArg("notification_channel_id is required")
	}

	// Validate notification_channel_id belongs to the user
	channel, errGetChannel := xs.db.GetNotificationChannelByID(notificationChannelID)
	if errGetChannel != nil {
		if errors.Is(errGetChannel, sql.ErrNoRows) {
			return "", "", "", invalidArg("notification channel not found")
		}
		log.Error().Err(errGetChannel).Str("channel_id", notificationChannelID).Msg("failed to fetch notification channel")
		return "", "", "", connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if channel.UserID != user.ID {
		return "", "", "", invalidArg("notification channel not found")
	}

	// Extract optional fields
	if serverIDPtr != nil {
		serverID = *serverIDPtr
	}
	if serverNodeIDPtr != nil {
		serverNodeID = *serverNodeIDPtr
	}
	if nodeIDPtr != nil {
		nodeID = *nodeIDPtr
	}

	// Pair consistency: serverID and serverNodeID must both be set or both empty
	if (serverID != "" && serverNodeID == "") || (serverID == "" && serverNodeID != "") {
		return "", "", "", invalidArg("server_id and server_node_id must both be provided or both omitted")
	}

	// Mutual exclusivity: serverID and nodeID cannot both be set
	if serverID != "" && nodeID != "" {
		return "", "", "", invalidArg("server_id and node_id are mutually exclusive")
	}

	// Event-type scoping validation
	if isNodeEventType(eventType) {
		if serverID != "" || serverNodeID != "" {
			return "", "", "", invalidArg("node event types must not have server_id/server_node_id")
		}
	}
	if isServerEventType(eventType) {
		if nodeID != "" {
			return "", "", "", invalidArg("server event types must not have node_id")
		}
	}

	// Server access check (if serverID provided)
	if serverID != "" {
		errAccess := xs.validateServerAccess(user, serverID, serverNodeID)
		if errAccess != nil {
			return "", "", "", errAccess
		}
	}

	return serverID, serverNodeID, nodeID, nil
}

// validateServerAccess verifies that the given server exists and that the
// user has access to it. In hub-spoke, game-server metadata is authoritative
// on the controller regardless of which node runs the process, so the
// local/remote distinction only matters for logging.
func (xs *XylonaService) validateServerAccess(user *models.User, serverID, serverNodeID string) error {
	gameServer, errServer := xs.getGameServerFromID(serverID)
	if errServer != nil {
		return errServer
	}
	// When the caller supplies a server_node_id, reject mismatches so the
	// controller-side state and the caller's expectation can't silently
	// disagree.
	if serverNodeID != "" && gameServer.NodeID != serverNodeID {
		return invalidArg("server_node_id does not match game server")
	}
	// Superusers and server owners always have access.
	if user.SuperUser || user.ID == gameServer.UserID {
		return nil
	}
	// Everyone else needs at least the base view permission on the server.
	errPerm := xs.ensureLocalServerPermission(user, gameServer, "game_server.view")
	if errPerm != nil {
		return errPerm
	}
	return nil
}

// CreateAlertRule creates a new alert rule for the caller.
func (xs *XylonaService) CreateAlertRule(
	_ context.Context,
	request *connect.Request[xylona.CreateAlertRuleRequest],
) (*connect.Response[xylona.CreateAlertRuleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	serverID, serverNodeID, nodeID, errValidate := xs.validateAlertRuleRequest(
		user,
		request.Msg.GetEventType(),
		request.Msg.GetNotificationChannelId(),
		request.Msg.ServerId,
		request.Msg.ServerNodeId,
		request.Msg.NodeId,
	)
	if errValidate != nil {
		return nil, errValidate
	}

	rule, errInsert := xs.db.InsertAlertRule(
		user.ID,
		serverID,
		serverNodeID,
		nodeID,
		request.Msg.GetEventType().String(),
		request.Msg.GetCondition(),
		request.Msg.GetNotificationChannelId(),
		request.Msg.GetEnabled(),
	)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("user_id", user.ID).Msg("failed to create alert rule")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create alert rule"))
	}

	return connect.NewResponse(&xylona.CreateAlertRuleResponse{
		Rule: alertRuleToProto(rule),
	}), nil
}

// UpdateAlertRule updates an existing alert rule.
func (xs *XylonaService) UpdateAlertRule(
	_ context.Context,
	request *connect.Request[xylona.UpdateAlertRuleRequest],
) (*connect.Response[xylona.UpdateAlertRuleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasGlobalPermission(user)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, invalidArg("id is required")
	}

	serverID, serverNodeID, nodeID, errValidate := xs.validateAlertRuleRequest(
		user,
		request.Msg.GetEventType(),
		request.Msg.GetNotificationChannelId(),
		request.Msg.ServerId,
		request.Msg.ServerNodeId,
		request.Msg.NodeId,
	)
	if errValidate != nil {
		return nil, errValidate
	}

	errUpdate := xs.db.UpdateAlertRule(
		id,
		user.ID,
		serverID,
		serverNodeID,
		nodeID,
		request.Msg.GetEventType().String(),
		request.Msg.GetCondition(),
		request.Msg.GetNotificationChannelId(),
		request.Msg.GetEnabled(),
	)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("alert_rule_id", id).Msg("failed to update alert rule")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to update alert rule"))
	}

	// Re-fetch the updated rule to return fresh data.
	updatedRule, errGet := xs.db.GetAlertRuleByID(id)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("alert rule not found"))
		}
		log.Error().Err(errGet).Str("alert_rule_id", id).Msg("failed to fetch alert rule after update")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch updated alert rule"))
	}
	// Verify ownership — the UPDATE was scoped by user_id, so if the rule
	// belongs to another user the update was a no-op.
	if updatedRule.UserID != user.ID {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("alert rule not found"))
	}

	return connect.NewResponse(&xylona.UpdateAlertRuleResponse{
		Rule: alertRuleToProto(updatedRule),
	}), nil
}

// DeleteAlertRule removes an alert rule by ID.
func (xs *XylonaService) DeleteAlertRule(
	_ context.Context,
	request *connect.Request[xylona.DeleteAlertRuleRequest],
) (*connect.Response[xylona.DeleteAlertRuleResponse], error) {
	errDelete := xs.deleteOwnedAlertResource(request.Header(), request.Msg.GetId(), alertResourceDeleteConfig{
		logIDField:       "alert_rule_id",
		deleteLogMessage: "failed to delete alert rule",
		internalMessage:  "failed to delete alert rule",
		deleteFn:         xs.db.DeleteAlertRule,
	})
	if errDelete != nil {
		return nil, errDelete
	}

	return connect.NewResponse(&xylona.DeleteAlertRuleResponse{}), nil
}

// ListAlertRules returns the alert rules visible to the caller.
func (xs *XylonaService) ListAlertRules(
	_ context.Context,
	request *connect.Request[xylona.ListAlertRulesRequest],
) (*connect.Response[xylona.ListAlertRulesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasAnyGlobalPermission(user, permissionAlertsManage, permissionAlertsViewHistory)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alert permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	// Pair consistency for server filter
	hasServerID := request.Msg.ServerId != nil
	hasServerNodeID := request.Msg.ServerNodeId != nil
	if hasServerID != hasServerNodeID {
		return nil, invalidArg("server_id and server_node_id must both be provided or both omitted")
	}

	var rules []*models.AlertRule

	// If server filter is provided, query by server
	switch {
	case hasServerID:
		serverID := request.Msg.GetServerId()
		serverNodeID := request.Msg.GetServerNodeId()

		// Verify access to the server
		errAccess := xs.validateServerAccess(user, serverID, serverNodeID)
		if errAccess != nil {
			return nil, errAccess
		}

		var errGet error
		rules, errGet = xs.db.GetAlertRulesByUserAndServerID(user.ID, serverID, serverNodeID)
		if errGet != nil {
			log.Error().Err(errGet).
				Str("user_id", user.ID).
				Str("server_id", serverID).
				Str("server_node_id", serverNodeID).
				Msg("failed to list alert rules by server")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list alert rules"))
		}
	default:
		var errGet error
		rules, errGet = xs.db.GetAlertRulesByUserID(user.ID)
		if errGet != nil {
			log.Error().Err(errGet).Str("user_id", user.ID).Msg("failed to list alert rules")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list alert rules"))
		}
	}

	resp := &xylona.ListAlertRulesResponse{}
	for _, r := range rules {
		resp.Rules = append(resp.Rules, alertRuleToProto(r))
	}

	return connect.NewResponse(resp), nil
}

// GetAlertHistory returns recent alert history entries visible to the caller.
func (xs *XylonaService) GetAlertHistory(
	_ context.Context,
	request *connect.Request[xylona.GetAlertHistoryRequest],
) (*connect.Response[xylona.GetAlertHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, unauthenticated()
	}

	allowed, errPerm := xs.hasAnyGlobalPermission(user, permissionAlertsManage, permissionAlertsViewHistory)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alert permissions")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, permissionDenied("insufficient permissions")
	}

	// Clamp limit to 1-100, default 50
	limit := int(request.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := max(int(request.Msg.GetOffset()), 0)

	// Pair consistency for server filter
	hasServerID := request.Msg.ServerId != nil
	hasServerNodeID := request.Msg.ServerNodeId != nil
	if hasServerID != hasServerNodeID {
		return nil, invalidArg("server_id and server_node_id must both be provided or both omitted")
	}

	var entries []*models.AlertHistory

	// If server filter is provided, query by server
	switch {
	case hasServerID:
		serverID := request.Msg.GetServerId()
		serverNodeID := request.Msg.GetServerNodeId()

		// Verify access to the server
		errAccess := xs.validateServerAccess(user, serverID, serverNodeID)
		if errAccess != nil {
			return nil, errAccess
		}

		var errGet error
		if user.SuperUser {
			entries, errGet = xs.db.GetAlertHistoryByServerID(serverID, serverNodeID, limit, offset)
		} else {
			entries, errGet = xs.db.GetAlertHistoryByUserAndServerID(user.ID, serverID, serverNodeID, limit, offset)
		}
		if errGet != nil {
			log.Error().Err(errGet).
				Str("user_id", user.ID).
				Str("server_id", serverID).
				Str("server_node_id", serverNodeID).
				Msg("failed to get alert history by server")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get alert history"))
		}
	case user.SuperUser:
		// Superusers see all history
		var errGet error
		entries, errGet = xs.db.GetAllAlertHistory(limit, offset)
		if errGet != nil {
			log.Error().Err(errGet).Msg("failed to get all alert history")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get alert history"))
		}
	default:
		var errGet error
		entries, errGet = xs.db.GetAlertHistoryByUserID(user.ID, limit, offset)
		if errGet != nil {
			log.Error().Err(errGet).Str("user_id", user.ID).Msg("failed to get alert history")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get alert history"))
		}
	}

	resp := &xylona.GetAlertHistoryResponse{}
	for _, h := range entries {
		resp.Entries = append(resp.Entries, alertHistoryToProto(h))
	}

	return connect.NewResponse(resp), nil
}

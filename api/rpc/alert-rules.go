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
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("event_type is required"))
	}

	// Validate notification_channel_id is not empty
	if strings.TrimSpace(notificationChannelID) == "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("notification_channel_id is required"))
	}

	// Validate notification_channel_id belongs to the user
	channel, errGetChannel := xs.db.GetNotificationChannelByID(notificationChannelID)
	if errGetChannel != nil {
		if errors.Is(errGetChannel, sql.ErrNoRows) {
			return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("notification channel not found"))
		}
		log.Error().Err(errGetChannel).Str("channel_id", notificationChannelID).Msg("failed to fetch notification channel")
		return "", "", "", connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if channel.UserID != user.ID {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("notification channel not found"))
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
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("server_id and server_node_id must both be provided or both omitted"))
	}

	// Mutual exclusivity: serverID and nodeID cannot both be set
	if serverID != "" && nodeID != "" {
		return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("server_id and node_id are mutually exclusive"))
	}

	// Event-type scoping validation
	if isNodeEventType(eventType) {
		if serverID != "" || serverNodeID != "" {
			return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("node event types must not have server_id/server_node_id"))
		}
	}
	if isServerEventType(eventType) {
		if nodeID != "" {
			return "", "", "", connect.NewError(connect.CodeInvalidArgument, errors.New("server event types must not have node_id"))
		}
	}

	// Server access check (if serverID provided)
	if serverID != "" {
		errAccess := xs.validateServerAccess(serverID, serverNodeID)
		if errAccess != nil {
			return "", "", "", errAccess
		}
	}

	return serverID, serverNodeID, nodeID, nil
}

// validateServerAccess verifies that the given server exists. For local
// servers it uses getGameServerFromID; for remote servers it verifies via
// the remote server cache.
func (xs *XylonaService) validateServerAccess(serverID, serverNodeID string) error {
	localNodeID, errLocal := xs.db.GetLocalNodeID()
	if errLocal != nil {
		log.Error().Err(errLocal).Msg("failed to get local node ID")
		return connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}

	if serverNodeID == localNodeID {
		// Local server — just verify it exists.
		_, errServer := xs.getGameServerFromID(serverID)
		if errServer != nil {
			return errServer
		}
		return nil
	}

	// Remote server — verify it exists in the remote cache.
	_, _, errRemote := xs.getRemoteNodeForServer(serverID)
	if errRemote != nil {
		return errRemote
	}
	return nil
}

func (xs *XylonaService) CreateAlertRule(
	ctx context.Context,
	request *connect.Request[xylona.CreateAlertRuleRequest],
) (*connect.Response[xylona.CreateAlertRuleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
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

func (xs *XylonaService) UpdateAlertRule(
	ctx context.Context,
	request *connect.Request[xylona.UpdateAlertRuleRequest],
) (*connect.Response[xylona.UpdateAlertRuleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
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
	rules, errGet := xs.db.GetAlertRulesByUserID(user.ID)
	if errGet != nil {
		log.Error().Err(errGet).Str("user_id", user.ID).Msg("failed to fetch alert rules after update")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to fetch updated alert rule"))
	}

	var updatedRule *models.AlertRule
	for _, r := range rules {
		if r.ID == id {
			updatedRule = r
			break
		}
	}
	if updatedRule == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("alert rule not found"))
	}

	return connect.NewResponse(&xylona.UpdateAlertRuleResponse{
		Rule: alertRuleToProto(updatedRule),
	}), nil
}

func (xs *XylonaService) DeleteAlertRule(
	ctx context.Context,
	request *connect.Request[xylona.DeleteAlertRuleRequest],
) (*connect.Response[xylona.DeleteAlertRuleResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	id := strings.TrimSpace(request.Msg.GetId())
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("id is required"))
	}

	errDelete := xs.db.DeleteAlertRule(id, user.ID)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("alert_rule_id", id).Msg("failed to delete alert rule")
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to delete alert rule"))
	}

	return connect.NewResponse(&xylona.DeleteAlertRuleResponse{}), nil
}

func (xs *XylonaService) ListAlertRules(
	ctx context.Context,
	request *connect.Request[xylona.ListAlertRulesRequest],
) (*connect.Response[xylona.ListAlertRulesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	var rules []*models.AlertRule

	// If server filter is provided, query by server
	if request.Msg.ServerId != nil && request.Msg.ServerNodeId != nil {
		serverID := *request.Msg.ServerId
		serverNodeID := *request.Msg.ServerNodeId

		// Verify access to the server
		errAccess := xs.validateServerAccess(serverID, serverNodeID)
		if errAccess != nil {
			return nil, errAccess
		}

		var errGet error
		rules, errGet = xs.db.GetAlertRulesByServerID(serverID, serverNodeID)
		if errGet != nil {
			log.Error().Err(errGet).
				Str("server_id", serverID).
				Str("server_node_id", serverNodeID).
				Msg("failed to list alert rules by server")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list alert rules"))
		}
	} else {
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

func (xs *XylonaService) GetAlertHistory(
	ctx context.Context,
	request *connect.Request[xylona.GetAlertHistoryRequest],
) (*connect.Response[xylona.GetAlertHistoryResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthenticated"))
	}

	allowed, errPerm := xs.hasGlobalPermission(user, permissionAlertsManage)
	if errPerm != nil {
		log.Error().Err(errPerm).Str("user_id", user.ID).Msg("failed to check alerts.manage permission")
		return nil, connect.NewError(connect.CodeInternal, errors.New("internal error"))
	}
	if !allowed {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("insufficient permissions"))
	}

	// Clamp limit to 1-100, default 50
	limit := int(request.Msg.GetLimit())
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset := int(request.Msg.GetOffset())
	if offset < 0 {
		offset = 0
	}

	var entries []*models.AlertHistory

	// If server filter is provided, query by server
	if request.Msg.ServerId != nil && request.Msg.ServerNodeId != nil {
		serverID := *request.Msg.ServerId
		serverNodeID := *request.Msg.ServerNodeId

		// Verify access to the server
		errAccess := xs.validateServerAccess(serverID, serverNodeID)
		if errAccess != nil {
			return nil, errAccess
		}

		var errGet error
		entries, errGet = xs.db.GetAlertHistoryByServerID(serverID, serverNodeID, limit, offset)
		if errGet != nil {
			log.Error().Err(errGet).
				Str("server_id", serverID).
				Str("server_node_id", serverNodeID).
				Msg("failed to get alert history by server")
			return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get alert history"))
		}
	} else {
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

package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// setNullableString sets an omitnull field to the given value or NULL if empty.
func setNullableString(value string) omitnull.Val[string] {
	if value == "" {
		return omitnull.FromNull[string](null.Val[string]{})
	}
	return omitnull.From(value)
}

// InsertAlertRule creates a new alert rule. Pass empty strings for serverID,
// serverNodeID, or nodeID to store NULL. The condition is also stored as NULL
// when empty.
func (c *Connection) InsertAlertRule(userID, serverID, serverNodeID, nodeID, eventType, condition, channelID string, enabled bool) (*models.AlertRule, error) {
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()
	id := uuid.New().String()

	setter := &models.AlertRuleSetter{
		ID:                    omit.From(id),
		UserID:                omit.From(userID),
		ServerID:              setNullableString(serverID),
		ServerNodeID:          setNullableString(serverNodeID),
		NodeID:                setNullableString(nodeID),
		EventType:             omit.From(eventType),
		Condition:             setNullableString(condition),
		NotificationChannelID: omit.From(channelID),
		Enabled:               omit.From(enabledInt),
		CreatedAt:             omit.From(now),
		UpdatedAt:             omit.From(now),
	}

	rule, errInsert := models.AlertRules.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting alert rule")
		return nil, fmt.Errorf("insert alert rule: %w", errInsert)
	}

	return rule, nil
}

// GetAlertRuleByID returns a single alert rule by its ID.
func (c *Connection) GetAlertRuleByID(id string) (*models.AlertRule, error) {
	rule, errGet := models.FindAlertRule(c.ctx, c.DB, id)
	if errGet != nil {
		return nil, fmt.Errorf("get alert rule by ID: %w", errGet)
	}
	return rule, nil
}

// GetAlertRulesByUserID returns all alert rules belonging to the given user.
func (c *Connection) GetAlertRulesByUserID(userID string) ([]*models.AlertRule, error) {
	rules, errGet := models.AlertRules.Query(
		models.SelectWhere.AlertRules.UserID.EQ(userID),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("user_id", userID).Msg("Error querying alert rules by user ID")
		return nil, fmt.Errorf("get alert rules by user ID: %w", errGet)
	}

	return rules, nil
}

// GetAlertRulesByServerID returns all alert rules matching the given
// server_id and server_node_id pair.
func (c *Connection) GetAlertRulesByServerID(serverID, serverNodeID string) ([]*models.AlertRule, error) {
	rules, errGet := models.AlertRules.Query(
		models.SelectWhere.AlertRules.ServerID.EQ(serverID),
		models.SelectWhere.AlertRules.ServerNodeID.EQ(serverNodeID),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("server_id", serverID).Str("server_node_id", serverNodeID).Msg("Error querying alert rules by server ID")
		return nil, fmt.Errorf("get alert rules by server ID: %w", errGet)
	}

	return rules, nil
}

// GetAlertRulesByUserAndServerID returns all alert rules owned by the given
// user that match the server_id and server_node_id pair.
func (c *Connection) GetAlertRulesByUserAndServerID(userID, serverID, serverNodeID string) ([]*models.AlertRule, error) {
	rules, errGet := models.AlertRules.Query(
		models.SelectWhere.AlertRules.UserID.EQ(userID),
		models.SelectWhere.AlertRules.ServerID.EQ(serverID),
		models.SelectWhere.AlertRules.ServerNodeID.EQ(serverNodeID),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).
			Str("user_id", userID).
			Str("server_id", serverID).
			Str("server_node_id", serverNodeID).
			Msg("Error querying alert rules by user and server ID")
		return nil, fmt.Errorf("get alert rules by user and server ID: %w", errGet)
	}

	return rules, nil
}

// GetEnabledAlertRulesByEventType returns all enabled alert rules that match
// the given event type.
func (c *Connection) GetEnabledAlertRulesByEventType(eventType string) ([]*models.AlertRule, error) {
	rules, errGet := models.AlertRules.Query(
		models.SelectWhere.AlertRules.EventType.EQ(eventType),
		models.SelectWhere.AlertRules.Enabled.EQ(1),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("event_type", eventType).Msg("Error querying enabled alert rules by event type")
		return nil, fmt.Errorf("get enabled alert rules by event type: %w", errGet)
	}

	return rules, nil
}

// CanDeliverServerAlert reports whether the rule owner may still receive
// events for the given game server.
func (c *Connection) CanDeliverServerAlert(userID string, serverID string) (bool, error) {
	user, errUser := c.GetUserByID(userID)
	if errUser != nil {
		if errors.Is(errUser, sql.ErrNoRows) {
			return false, nil
		}
		return false, errUser
	}
	gameServer, errServer := c.GetGameServerByID(serverID)
	if errServer != nil {
		if errors.Is(errServer, sql.ErrNoRows) {
			return false, nil
		}
		return false, errServer
	}
	return HasPermission(c, user, serverID, gameServer.UserID, "game_server.view")
}

// CanDeliverNodeAlert reports whether the rule owner may still receive node
// resource alerts. Live node snapshots are superuser-only.
func (c *Connection) CanDeliverNodeAlert(userID string) (bool, error) {
	user, errUser := c.GetUserByID(userID)
	if errUser != nil {
		if errors.Is(errUser, sql.ErrNoRows) {
			return false, nil
		}
		return false, errUser
	}
	return user.SuperUser, nil
}

// UpdateAlertRule updates all mutable fields of an alert rule identified by id
// and scoped to userID.
func (c *Connection) UpdateAlertRule(id, userID, serverID, serverNodeID, nodeID, eventType, condition, channelID string, enabled bool) error {
	enabledInt := int64(0)
	if enabled {
		enabledInt = 1
	}

	now := time.Now().UTC()

	setter := &models.AlertRuleSetter{
		ServerID:              setNullableString(serverID),
		ServerNodeID:          setNullableString(serverNodeID),
		NodeID:                setNullableString(nodeID),
		EventType:             omit.From(eventType),
		Condition:             setNullableString(condition),
		NotificationChannelID: omit.From(channelID),
		Enabled:               omit.From(enabledInt),
		UpdatedAt:             omit.From(now),
	}

	_, errUpdate := models.AlertRules.Update(
		setter.UpdateMod(),
		models.UpdateWhere.AlertRules.ID.EQ(id),
		models.UpdateWhere.AlertRules.UserID.EQ(userID),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("alert_rule_id", id).Msg("Error updating alert rule")
		return fmt.Errorf("update alert rule: %w", errUpdate)
	}

	return nil
}

// DeleteAlertRule deletes the alert rule identified by id and scoped to
// userID. Scoping the delete to userID prevents cross-user deletion.
func (c *Connection) DeleteAlertRule(id, userID string) error {
	_, errDelete := models.AlertRules.Delete(
		models.DeleteWhere.AlertRules.ID.EQ(id),
		models.DeleteWhere.AlertRules.UserID.EQ(userID),
	).Exec(c.ctx, c.DB)
	if errDelete != nil {
		log.Error().Err(errDelete).Str("alert_rule_id", id).Msg("Error deleting alert rule")
		return fmt.Errorf("delete alert rule: %w", errDelete)
	}
	return nil
}

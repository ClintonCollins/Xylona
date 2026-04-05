package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/aarondl/opt/omit"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite/sm"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// InsertAlertHistory creates a new alert history record. Pass empty strings
// for nullable fields (ruleID, serverID, serverNodeID, nodeID, eventData) to
// store NULL.
func (c *Connection) InsertAlertHistory(ruleID, userID, serverID, serverNodeID, nodeID, eventType, eventData, channelType, deliveryStatus string) (*models.AlertHistory, error) {
	now := time.Now().UTC()
	id := uuid.New().String()

	setter := &models.AlertHistorySetter{
		ID:             omit.From(id),
		AlertRuleID:    setNullableString(ruleID),
		UserID:         omit.From(userID),
		ServerID:       setNullableString(serverID),
		ServerNodeID:   setNullableString(serverNodeID),
		NodeID:         setNullableString(nodeID),
		EventType:      omit.From(eventType),
		EventData:      setNullableString(eventData),
		ChannelType:    omit.From(channelType),
		DeliveryStatus: omit.From(deliveryStatus),
		CreatedAt:      omit.From(now),
	}

	history, errInsert := models.AlertHistories.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Msg("Error inserting alert history")
		return nil, fmt.Errorf("insert alert history: %w", errInsert)
	}

	return history, nil
}

// UpdateAlertHistoryDeliveryStatus updates the delivery_status and optionally
// the delivery_error of an alert history record. Pass an empty deliveryError
// to store NULL.
func (c *Connection) UpdateAlertHistoryDeliveryStatus(id, status, deliveryError string) error {
	setter := &models.AlertHistorySetter{
		DeliveryStatus: omit.From(status),
		DeliveryError:  setNullableString(deliveryError),
	}

	_, errUpdate := models.AlertHistories.Update(
		setter.UpdateMod(),
		models.UpdateWhere.AlertHistories.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("alert_history_id", id).Msg("Error updating alert history delivery status")
		return fmt.Errorf("update alert history delivery status: %w", errUpdate)
	}

	return nil
}

// GetAlertHistoryByUserID returns alert history records for the given user,
// ordered by created_at descending with pagination.
func (c *Connection) GetAlertHistoryByUserID(userID string, limit, offset int) ([]*models.AlertHistory, error) {
	results, errGet := models.AlertHistories.Query(
		models.SelectWhere.AlertHistories.UserID.EQ(userID),
		sm.OrderBy(models.AlertHistories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("user_id", userID).Msg("Error querying alert history by user ID")
		return nil, fmt.Errorf("get alert history by user ID: %w", errGet)
	}

	return results, nil
}

// GetAlertHistoryByServerID returns alert history records for the given
// server_id and server_node_id pair, ordered by created_at descending with
// pagination.
func (c *Connection) GetAlertHistoryByServerID(serverID, serverNodeID string, limit, offset int) ([]*models.AlertHistory, error) {
	results, errGet := models.AlertHistories.Query(
		models.SelectWhere.AlertHistories.ServerID.EQ(serverID),
		models.SelectWhere.AlertHistories.ServerNodeID.EQ(serverNodeID),
		sm.OrderBy(models.AlertHistories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Str("server_id", serverID).Str("server_node_id", serverNodeID).Msg("Error querying alert history by server ID")
		return nil, fmt.Errorf("get alert history by server ID: %w", errGet)
	}

	return results, nil
}

// GetAlertHistoryByUserAndServerID returns alert history records for the given
// user and server_id/server_node_id pair, ordered by created_at descending with
// pagination.
func (c *Connection) GetAlertHistoryByUserAndServerID(userID, serverID, serverNodeID string, limit, offset int) ([]*models.AlertHistory, error) {
	results, errGet := models.AlertHistories.Query(
		models.SelectWhere.AlertHistories.UserID.EQ(userID),
		models.SelectWhere.AlertHistories.ServerID.EQ(serverID),
		models.SelectWhere.AlertHistories.ServerNodeID.EQ(serverNodeID),
		sm.OrderBy(models.AlertHistories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).
			Str("user_id", userID).
			Str("server_id", serverID).
			Str("server_node_id", serverNodeID).
			Msg("Error querying alert history by user and server ID")
		return nil, fmt.Errorf("get alert history by user and server ID: %w", errGet)
	}

	return results, nil
}

// GetAllAlertHistory returns all alert history records (regardless of user),
// ordered by created_at descending with pagination. Intended for superuser access.
func (c *Connection) GetAllAlertHistory(limit, offset int) ([]*models.AlertHistory, error) {
	results, errGet := models.AlertHistories.Query(
		sm.OrderBy(models.AlertHistories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, nil
		}
		log.Error().Err(errGet).Msg("Error querying all alert history")
		return nil, fmt.Errorf("get all alert history: %w", errGet)
	}

	return results, nil
}

// PruneAlertHistory deletes alert history records older than the given time
// and returns the number of deleted rows.
func (c *Connection) PruneAlertHistory(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(c.ctx,
		`DELETE FROM alert_history WHERE created_at < ?`,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error pruning alert history")
		return 0, fmt.Errorf("prune alert history: %w", errExec)
	}
	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("prune alert history rows affected: %w", errRowsAffected)
	}
	return rowsAffected, nil
}

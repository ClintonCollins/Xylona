package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/stephenafamo/bob/dialect/sqlite/im"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetOrCreateAlertState returns the existing alert state for the given
// rule/entity combination, or creates a new one if none exists. The unique
// index on (alert_rule_id, entity_type, entity_id, entity_node_id) ensures
// at most one row per combination.
func (c *Connection) GetOrCreateAlertState(ruleID, entityType, entityID, entityNodeID string) (*models.AlertState, error) {
	id := uuid.New().String()
	setter := &models.AlertStateSetter{
		ID:           omit.From(id),
		AlertRuleID:  omit.From(ruleID),
		EntityType:   omit.From(entityType),
		EntityID:     omit.From(entityID),
		EntityNodeID: omit.From(entityNodeID),
		Triggered:    omit.From(int64(0)),
	}

	_, errInsert := models.AlertStates.Insert(
		im.OnConflict(
			models.AlertStates.Columns.AlertRuleID,
			models.AlertStates.Columns.EntityType,
			models.AlertStates.Columns.EntityID,
			models.AlertStates.Columns.EntityNodeID,
		).DoNothing(),
		setter,
	).Exec(c.ctx, c.DB)
	if errInsert != nil {
		log.Error().Err(errInsert).Str("alert_rule_id", ruleID).Msg("Error inserting alert state")
		return nil, errInsert
	}

	existing, errGet := models.AlertStates.Query(
		models.SelectWhere.AlertStates.AlertRuleID.EQ(ruleID),
		models.SelectWhere.AlertStates.EntityType.EQ(entityType),
		models.SelectWhere.AlertStates.EntityID.EQ(entityID),
		models.SelectWhere.AlertStates.EntityNodeID.EQ(entityNodeID),
	).One(c.ctx, c.DB)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		log.Error().Err(errGet).Str("alert_rule_id", ruleID).Msg("Error querying alert state")
		return nil, errGet
	}

	return existing, nil
}

// UpdateAlertStateTriggered sets the triggered flag on an alert state. When
// triggered is true, triggered_at is set to now and resolved_at is cleared.
// When triggered is false, triggered_at is preserved and resolved_at is set
// to now.
func (c *Connection) UpdateAlertStateTriggered(id string, triggered bool) error {
	now := time.Now().UTC()

	var setter *models.AlertStateSetter
	if triggered {
		setter = &models.AlertStateSetter{
			Triggered:   omit.From(int64(1)),
			TriggeredAt: omitnull.From(now),
			ResolvedAt:  omitnull.FromNull[time.Time](null.Val[time.Time]{}),
		}
	} else {
		setter = &models.AlertStateSetter{
			Triggered:  omit.From(int64(0)),
			ResolvedAt: omitnull.From(now),
		}
	}

	_, errUpdate := models.AlertStates.Update(
		setter.UpdateMod(),
		models.UpdateWhere.AlertStates.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if errUpdate != nil {
		log.Error().Err(errUpdate).Str("alert_state_id", id).Bool("triggered", triggered).Msg("Error updating alert state triggered")
		return errUpdate
	}

	return nil
}

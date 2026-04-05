package db

import (
	"fmt"
	"strings"

	"github.com/aarondl/opt/omit"
	"github.com/stephenafamo/bob/dialect/sqlite"
	"github.com/stephenafamo/bob/dialect/sqlite/sm"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// InsertFederationAdvisory inserts a new federation advisory record.
func (c *Connection) InsertFederationAdvisory(setter *models.FederationAdvisorySetter) (*models.FederationAdvisory, error) {
	advisory, errInsert := models.FederationAdvisories.Insert(setter).One(c.ctx, c.DB)
	if errInsert != nil {
		return nil, fmt.Errorf("insert federation advisory: %w", errInsert)
	}
	return advisory, nil
}

// ListFederationAdvisories returns federation advisories with optional unread
// filter and pagination. Returns the matching advisories and total count.
func (c *Connection) ListFederationAdvisories(unreadOnly bool, limit, offset int) ([]*models.FederationAdvisory, int64, error) {
	if unreadOnly {
		return c.listAdvisoriesFiltered(limit, offset)
	}
	return c.listAdvisoriesAll(limit, offset)
}

func (c *Connection) listAdvisoriesAll(limit, offset int) ([]*models.FederationAdvisory, int64, error) {
	total, errCount := models.FederationAdvisories.Query().Count(c.ctx, c.DB)
	if errCount != nil {
		return nil, 0, fmt.Errorf("count federation advisories: %w", errCount)
	}

	advisories, errQuery := models.FederationAdvisories.Query(
		sm.OrderBy(models.FederationAdvisories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errQuery != nil {
		return nil, 0, fmt.Errorf("list federation advisories: %w", errQuery)
	}

	return advisories, total, nil
}

func (c *Connection) listAdvisoriesFiltered(limit, offset int) ([]*models.FederationAdvisory, int64, error) {
	unreadFilter := models.SelectWhere.FederationAdvisories.Read.EQ(false)

	total, errCount := models.FederationAdvisories.Query(unreadFilter).Count(c.ctx, c.DB)
	if errCount != nil {
		return nil, 0, fmt.Errorf("count unread federation advisories: %w", errCount)
	}

	advisories, errQuery := models.FederationAdvisories.Query(
		unreadFilter,
		sm.OrderBy(models.FederationAdvisories.Columns.CreatedAt).Desc(),
		sm.Limit(limit),
		sm.Offset(offset),
	).All(c.ctx, c.DB)
	if errQuery != nil {
		return nil, 0, fmt.Errorf("list unread federation advisories: %w", errQuery)
	}

	return advisories, total, nil
}

// MarkAdvisoriesRead marks advisories as read. If ids is nil or empty, all
// advisories are marked as read. Otherwise only the specified IDs are updated.
func (c *Connection) MarkAdvisoriesRead(ids []string) error {
	readSetter := models.FederationAdvisorySetter{
		Read: omit.From(true),
	}

	if len(ids) == 0 {
		_, errExec := models.FederationAdvisories.Update(
			readSetter.UpdateMod(),
			models.UpdateWhere.FederationAdvisories.Read.EQ(false),
		).All(c.ctx, c.DB)
		if errExec != nil {
			return fmt.Errorf("mark all federation advisories read: %w", errExec)
		}
		return nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(
		`UPDATE federation_advisory SET read = 1 WHERE id IN (%s)`,
		strings.Join(placeholders, ", "),
	)
	_, errExec := sqlite.RawQuery(query, args...).Exec(c.ctx, c.DB)
	if errExec != nil {
		return fmt.Errorf("mark federation advisories read: %w", errExec)
	}
	return nil
}

// GetUnreadAdvisoryCount returns the number of unread federation advisories.
func (c *Connection) GetUnreadAdvisoryCount() (int64, error) {
	count, errCount := models.FederationAdvisories.Query(
		models.SelectWhere.FederationAdvisories.Read.EQ(false),
	).Count(c.ctx, c.DB)
	if errCount != nil {
		return 0, fmt.Errorf("count unread federation advisories: %w", errCount)
	}
	return count, nil
}

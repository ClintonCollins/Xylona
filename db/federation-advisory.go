package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/stephenafamo/bob/dialect/sqlite"
)

// FederationAdvisory represents a federation advisory record.
// NOTE: This is a temporary hand-written struct. Once bob is upgraded to a
// version that compiles on Go 1.26, regenerate models and switch to
// *models.FederationAdvisory / *models.FederationAdvisorySetter.
type FederationAdvisory struct {
	ID                 string
	Type               string
	Title              string
	Message            string
	SourceNodeID       string
	SourceNodeName     string
	SubjectNodeID      string
	SubjectNodeName    string
	SubjectNodeBaseURL string
	Read               bool
	CreatedAt          time.Time
}

// InsertFederationAdvisory inserts a new federation advisory record.
func (c *Connection) InsertFederationAdvisory(advisory FederationAdvisory) error {
	_, errInsert := sqlite.RawQuery(
		`INSERT INTO federation_advisory
			(id, type, title, message, source_node_id, source_node_name,
			 subject_node_id, subject_node_name, subject_node_base_url, read)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		advisory.ID,
		advisory.Type,
		advisory.Title,
		advisory.Message,
		advisory.SourceNodeID,
		advisory.SourceNodeName,
		advisory.SubjectNodeID,
		advisory.SubjectNodeName,
		advisory.SubjectNodeBaseURL,
		advisory.Read,
	).Exec(c.ctx, c.DB)
	return errInsert
}

// ListFederationAdvisories returns federation advisories with optional unread
// filter and pagination. Returns the matching advisories and total count.
func (c *Connection) ListFederationAdvisories(unreadOnly bool, limit, offset int) ([]FederationAdvisory, int, error) {
	// Count total matching rows.
	countQuery := `SELECT COUNT(*) FROM federation_advisory`
	if unreadOnly {
		countQuery += ` WHERE read = 0`
	}
	var total int
	errCount := c.SQLDb.QueryRowContext(c.ctx, countQuery).Scan(&total)
	if errCount != nil {
		return nil, 0, errCount
	}

	// Fetch paginated results.
	selectQuery := `SELECT id, type, title, message, source_node_id, source_node_name,
		subject_node_id, subject_node_name, subject_node_base_url, read, created_at
		FROM federation_advisory`
	if unreadOnly {
		selectQuery += ` WHERE read = 0`
	}
	selectQuery += ` ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, errQuery := c.SQLDb.QueryContext(c.ctx, selectQuery, limit, offset)
	if errQuery != nil {
		return nil, 0, errQuery
	}
	defer rows.Close()

	var advisories []FederationAdvisory
	for rows.Next() {
		var a FederationAdvisory
		errScan := rows.Scan(
			&a.ID, &a.Type, &a.Title, &a.Message,
			&a.SourceNodeID, &a.SourceNodeName,
			&a.SubjectNodeID, &a.SubjectNodeName, &a.SubjectNodeBaseURL,
			&a.Read, &a.CreatedAt,
		)
		if errScan != nil {
			return nil, 0, errScan
		}
		advisories = append(advisories, a)
	}
	if errRows := rows.Err(); errRows != nil {
		return nil, 0, errRows
	}

	return advisories, total, nil
}

// MarkAdvisoriesRead marks advisories as read. If ids is nil or empty, all
// advisories are marked as read. Otherwise only the specified IDs are updated.
func (c *Connection) MarkAdvisoriesRead(ids []string) error {
	if len(ids) == 0 {
		_, errExec := sqlite.RawQuery(
			`UPDATE federation_advisory SET read = 1 WHERE read = 0`,
		).Exec(c.ctx, c.DB)
		return errExec
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
	return errExec
}

// GetUnreadAdvisoryCount returns the number of unread federation advisories.
func (c *Connection) GetUnreadAdvisoryCount() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx,
		`SELECT COUNT(*) FROM federation_advisory WHERE read = 0`,
	).Scan(&count)
	if errQuery != nil {
		return 0, errQuery
	}
	return count, nil
}

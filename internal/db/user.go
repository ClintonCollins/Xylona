package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

// GetUser returns a user by username.
func (c *Connection) GetUser(username string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.UserName.EQ(username)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// GetUserByID returns a user by ID.
func (c *Connection) GetUserByID(id string) (*models.User, error) {
	user, err := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user")
		}
		return nil, fmt.Errorf("get user by ID: %w", err)
	}
	return user, nil
}

// CreateUser inserts a new user record.
func (c *Connection) CreateUser(userSetter *models.UserSetter) (*models.User, error) {
	user, err := models.Users.Insert(userSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user")
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// UpdateUser updates an existing user record.
func (c *Connection) UpdateUser(userSetter *models.UserSetter) error {
	_, err := models.Users.Update(
		userSetter.UpdateMod(),
		models.UpdateWhere.Users.ID.EQ(userSetter.ID.MustGet()),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error updating user")
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

// CreateUserSession inserts a new user session record.
func (c *Connection) CreateUserSession(userSessionSetter *models.UserSessionSetter) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Insert(userSessionSetter).One(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error creating user session")
		return nil, fmt.Errorf("create user session: %w", err)
	}
	return userSession, nil
}

// GetUserSession returns a user session by ID.
func (c *Connection) GetUserSession(id string) (*models.UserSession, error) {
	userSession, err := models.UserSessions.Query(models.SelectWhere.UserSessions.ID.EQ(id)).One(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying user session")
		}
		return nil, fmt.Errorf("get user session: %w", err)
	}
	return userSession, nil
}

// DeleteUserSession deletes a user session by ID.
func (c *Connection) DeleteUserSession(id string) error {
	_, err := models.UserSessions.Delete(
		models.DeleteWhere.UserSessions.ID.EQ(id),
	).Exec(c.ctx, c.DB)
	if err != nil {
		log.Error().Err(err).Msg("Error deleting user session")
		return fmt.Errorf("delete user session: %w", err)
	}
	return nil
}

// DeleteUserSessionsByUserID deletes every session belonging to userID.
func (c *Connection) DeleteUserSessionsByUserID(userID string) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`DELETE FROM user_session WHERE user_id = ?`,
		userID,
	)
	if errExec != nil {
		log.Error().Err(errExec).Str("user_id", userID).Msg("Error deleting user sessions")
		return 0, fmt.Errorf("delete user sessions by user ID: %w", errExec)
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("delete user sessions by user ID rows affected: %w", errRowsAffected)
	}

	return rowsAffected, nil
}

// TouchUserSession records recent activity on a session.
func (c *Connection) TouchUserSession(id string, at time.Time) error {
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`UPDATE user_session SET updated_at = ? WHERE id = ?`,
		at.UTC().Format("2006-01-02 15:04:05"),
		id,
	)
	if errExec != nil {
		log.Error().Err(errExec).Str("session_id", id).Msg("Error touching user session")
		return fmt.Errorf("touch user session: %w", errExec)
	}
	return nil
}

// PruneExpiredUserSessions deletes user sessions that expired before the given time.
func (c *Connection) PruneExpiredUserSessions(olderThan time.Time) (int64, error) {
	result, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`DELETE FROM user_session WHERE expires_at < ?`,
		olderThan.UTC().Format("2006-01-02 15:04:05"),
	)
	if errExec != nil {
		log.Error().Err(errExec).Msg("Error pruning expired user sessions")
		return 0, fmt.Errorf("prune expired user sessions: %w", errExec)
	}

	rowsAffected, errRowsAffected := result.RowsAffected()
	if errRowsAffected != nil {
		return 0, fmt.Errorf("prune expired user sessions rows affected: %w", errRowsAffected)
	}

	return rowsAffected, nil
}

// GetAllUsers returns all user records.
func (c *Connection) GetAllUsers() ([]*models.User, error) {
	users, err := models.Users.Query().All(c.ctx, c.DB)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Error().Err(err).Msg("Error querying users")
		}
		return nil, fmt.Errorf("get all users: %w", err)
	}
	return users, nil
}

// CountSuperUsers returns the number of superuser accounts.
func (c *Connection) CountSuperUsers() (int, error) {
	var count int
	errQuery := c.SQLDb.QueryRowContext(c.ctx, `SELECT COUNT(*) FROM user WHERE super_user = 1`).Scan(&count)
	if errQuery != nil {
		log.Error().Err(errQuery).Msg("Error counting super users")
		return 0, fmt.Errorf("count super users: %w", errQuery)
	}
	return count, nil
}

// DeleteUser deletes a user by ID.
func (c *Connection) DeleteUser(id string) error {
	user, errGetUser := models.Users.Query(models.SelectWhere.Users.ID.EQ(id)).One(c.ctx, c.DB)
	if errGetUser != nil {
		if !errors.Is(errGetUser, sql.ErrNoRows) {
			log.Error().Err(errGetUser).Str("user_id", id).Msg("Error querying user for delete")
		}
		return fmt.Errorf("get user for delete: %w", errGetUser)
	}

	errDeleteUser := models.UserSlice{user}.DeleteAll(c.ctx, c.DB)
	if errDeleteUser != nil {
		log.Error().Err(errDeleteUser).Str("user_id", id).Msg("Error deleting user")
		return fmt.Errorf("delete user: %w", errDeleteUser)
	}

	return nil
}

// UserDeletionSchedule is a scheduled task that is deleted along with the user
// who created it.
type UserDeletionSchedule struct {
	ID             string
	GameServerID   string
	GameServerName string
	Name           string
}

// UserDeletionGameServer is a game server that blocks deleting its owner.
type UserDeletionGameServer struct {
	ID   string
	Name string
}

// UserAccessGrant is game server access one user granted to another. An empty
// game server means the grant covers every game server.
type UserAccessGrant struct {
	GameServerID   string
	GameServerName string
	UserName       string
}

// UserDeletionImpact lists what deleting a user removes and what blocks it.
type UserDeletionImpact struct {
	Schedules        []UserDeletionSchedule
	OwnedGameServers []UserDeletionGameServer
	GrantsGiven      []UserAccessGrant
}

// GetUserDeletionImpact returns the schedules that cascade with the user, and
// the owned game servers and grants to others that block the delete.
func (c *Connection) GetUserDeletionImpact(userID string) (*UserDeletionImpact, error) {
	impact := &UserDeletionImpact{}

	scheduleRows, errSchedules := c.queryStringRows(
		`select st.id, st.game_server_id, gs.name, st.name
		 from scheduled_task st
		 join game_server gs on gs.id = st.game_server_id
		 where st.created_by = ?
		 order by gs.name, st.name`,
		userID,
	)
	if errSchedules != nil {
		return nil, fmt.Errorf("get schedules for user deletion: %w", errSchedules)
	}
	for _, row := range scheduleRows {
		impact.Schedules = append(impact.Schedules, UserDeletionSchedule{
			ID: row[0], GameServerID: row[1], GameServerName: row[2], Name: row[3],
		})
	}

	serverRows, errServers := c.queryStringRows(
		`select id, name from game_server where user_id = ? order by name`,
		userID,
	)
	if errServers != nil {
		return nil, fmt.Errorf("get owned game servers for user deletion: %w", errServers)
	}
	for _, row := range serverRows {
		impact.OwnedGameServers = append(impact.OwnedGameServers, UserDeletionGameServer{ID: row[0], Name: row[1]})
	}

	// Grants the user gave themselves cascade with them, so only grants to
	// others block the delete.
	grantRows, errGrants := c.queryStringRows(
		`select coalesce(gs.id, ''), coalesce(gs.name, ''), u.user_name
		 from user_role_assignment ura
		 join user u on u.id = ura.user_id
		 left join game_server gs on gs.id = ura.game_server_id
		 where ura.granted_by = ? and ura.user_id <> ?
		 order by gs.name, u.user_name`,
		userID, userID,
	)
	if errGrants != nil {
		return nil, fmt.Errorf("get access grants for user deletion: %w", errGrants)
	}
	for _, row := range grantRows {
		impact.GrantsGiven = append(impact.GrantsGiven, UserAccessGrant{
			GameServerID: row[0], GameServerName: row[1], UserName: row[2],
		})
	}

	return impact, nil
}

// queryStringRows runs a query whose columns are all text and returns each row
// as a slice of column values.
func (c *Connection) queryStringRows(query string, args ...any) ([][]string, error) {
	rows, errQuery := c.SQLDb.QueryContext(c.ctx, query, args...)
	if errQuery != nil {
		return nil, fmt.Errorf("query: %w", errQuery)
	}
	defer func() {
		errClose := rows.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close query rows")
		}
	}()

	columns, errColumns := rows.Columns()
	if errColumns != nil {
		return nil, fmt.Errorf("read columns: %w", errColumns)
	}

	var result [][]string
	for rows.Next() {
		values := make([]string, len(columns))
		targets := make([]any, len(values))
		for i := range values {
			targets[i] = &values[i]
		}
		errScan := rows.Scan(targets...)
		if errScan != nil {
			return nil, fmt.Errorf("scan row: %w", errScan)
		}
		result = append(result, values)
	}
	errRows := rows.Err()
	if errRows != nil {
		return nil, fmt.Errorf("iterate rows: %w", errRows)
	}

	return result, nil
}

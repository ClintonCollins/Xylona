package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/ClintonCollins/Xylona/sql/models/dberrors"
)

// ErrDNSRecordBindingTargetConflict indicates that another binding owns the target record.
var ErrDNSRecordBindingTargetConflict = errors.New("DNS record binding target is already owned")

// DNSRecordBinding stores one game server's requested relative name and optional ownership snapshot.
type DNSRecordBinding struct {
	GameServerID string
	RelativeName string
	Ownership    *DNSRecordOwnership
}

// DNSRecordOwnership is the material provider record state last accepted by Xylona.
type DNSRecordOwnership struct {
	ProviderRecordID *string
	FQDN             string
	RecordType       string
	Value            string
	TTL              int32
}

// GetDNSRecordBinding returns the binding for one game server.
func (c *Connection) GetDNSRecordBinding(gameServerID string) (*DNSRecordBinding, error) {
	row := c.SQLDb.QueryRowContext(
		c.ctx,
		`select game_server_id, relative_name, owned_provider_record_id, owned_fqdn,
		        owned_record_type, owned_value, owned_ttl
		 from dns_record_binding
		 where game_server_id = ?`,
		gameServerID,
	)
	return scanDNSRecordBinding(row)
}

// UpsertDNSRecordBinding stores a relative name without changing the ownership snapshot.
func (c *Connection) UpsertDNSRecordBinding(gameServerID string, relativeName string) error {
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`insert into dns_record_binding (game_server_id, relative_name)
		 values (?, ?)
		 on conflict(game_server_id) do update set relative_name = excluded.relative_name`,
		gameServerID,
		relativeName,
	)
	if errExec != nil {
		return fmt.Errorf("upsert DNS record binding: %w", errExec)
	}
	return nil
}

// RemoveDNSRecordBinding removes only the local binding and ownership snapshot.
func (c *Connection) RemoveDNSRecordBinding(gameServerID string) error {
	_, errExec := c.SQLDb.ExecContext(
		c.ctx,
		`delete from dns_record_binding where game_server_id = ?`,
		gameServerID,
	)
	if errExec != nil {
		return fmt.Errorf("remove DNS record binding: %w", errExec)
	}
	return nil
}

// CountDNSRecordBindings returns the number of configured bindings.
func (c *Connection) CountDNSRecordBindings() (int, error) {
	var count int
	errScan := c.SQLDb.QueryRowContext(c.ctx, `select count(*) from dns_record_binding`).Scan(&count)
	if errScan != nil {
		return 0, fmt.Errorf("count DNS record bindings: %w", errScan)
	}
	return count, nil
}

// DNSRecordBindingTargetOwned reports whether another binding owns a record target.
func (c *Connection) DNSRecordBindingTargetOwned(gameServerID string, fqdn string, recordType string) (bool, error) {
	var owned bool
	errScan := c.SQLDb.QueryRowContext(
		c.ctx,
		`select exists(
		 select 1 from dns_record_binding
		 where game_server_id != ? and owned_fqdn = ? and owned_record_type = ?
		)`,
		gameServerID,
		fqdn,
		recordType,
	).Scan(&owned)
	if errScan != nil {
		return false, fmt.Errorf("check DNS record binding target ownership: %w", errScan)
	}
	return owned, nil
}

// ReplaceDNSRecordBindingOwnership atomically replaces all ownership snapshot fields.
func (c *Connection) ReplaceDNSRecordBindingOwnership(gameServerID string, ownership DNSRecordOwnership) error {
	var updatedGameServerID string
	errScan := c.SQLDb.QueryRowContext(
		c.ctx,
		`update dns_record_binding
		 set owned_provider_record_id = ?, owned_fqdn = ?, owned_record_type = ?,
		     owned_value = ?, owned_ttl = ?
		 where game_server_id = ?
		 returning game_server_id`,
		nullableStringPointer(ownership.ProviderRecordID),
		ownership.FQDN,
		ownership.RecordType,
		ownership.Value,
		ownership.TTL,
		gameServerID,
	).Scan(&updatedGameServerID)
	if dberrors.ErrUniqueConstraint.Is(errScan) {
		return ErrDNSRecordBindingTargetConflict
	}
	if errScan != nil {
		return fmt.Errorf("replace DNS record binding ownership: %w", errScan)
	}
	return nil
}

// ClearDNSRecordBindingOwnership atomically clears all ownership snapshot fields.
func (c *Connection) ClearDNSRecordBindingOwnership(gameServerID string) error {
	var updatedGameServerID string
	errScan := c.SQLDb.QueryRowContext(
		c.ctx,
		`update dns_record_binding
		 set owned_provider_record_id = null, owned_fqdn = null, owned_record_type = null,
		     owned_value = null, owned_ttl = null
		 where game_server_id = ?
		 returning game_server_id`,
		gameServerID,
	).Scan(&updatedGameServerID)
	if errScan != nil {
		return fmt.Errorf("clear DNS record binding ownership: %w", errScan)
	}
	return nil
}

func scanDNSRecordBinding(row scanner) (*DNSRecordBinding, error) {
	var binding DNSRecordBinding
	var providerRecordID sql.NullString
	var fqdn sql.NullString
	var recordType sql.NullString
	var value sql.NullString
	var ttl sql.NullInt32
	errScan := row.Scan(
		&binding.GameServerID,
		&binding.RelativeName,
		&providerRecordID,
		&fqdn,
		&recordType,
		&value,
		&ttl,
	)
	if errScan != nil {
		return nil, fmt.Errorf("scan DNS record binding: %w", errScan)
	}
	if fqdn.Valid {
		binding.Ownership = &DNSRecordOwnership{
			ProviderRecordID: nullableStringValue(providerRecordID),
			FQDN:             fqdn.String,
			RecordType:       recordType.String,
			Value:            value.String,
			TTL:              ttl.Int32,
		}
	}
	return &binding, nil
}

func nullableStringPointer(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

func nullableStringValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return new(value.String)
}

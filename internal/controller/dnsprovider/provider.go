// Package dnsprovider defines the controller's minimal DNS provider contract.
package dnsprovider

import (
	"context"
	"errors"
)

var (
	// ErrUnauthorized reports invalid or missing provider credentials.
	ErrUnauthorized = errors.New("dns provider unauthorized")
	// ErrForbidden reports credentials without permission for the operation.
	ErrForbidden = errors.New("dns provider forbidden")
	// ErrNotFound reports a missing provider resource.
	ErrNotFound = errors.New("dns provider resource not found")
	// ErrConflict reports a provider-side state conflict.
	ErrConflict = errors.New("dns provider conflict")
	// ErrUnavailable reports a provider or transport failure.
	ErrUnavailable = errors.New("dns provider unavailable")
	// ErrUnsupported reports a record shape that cannot be managed safely.
	ErrUnsupported = errors.New("dns record shape unsupported")
)

// RecordType identifies a supported DNS record type.
type RecordType string

// Supported record types.
const (
	RecordTypeA    RecordType = "A"
	RecordTypeAAAA RecordType = "AAAA"
)

// Zone identifies an authoritative provider zone.
type Zone struct {
	ID   string
	Name string
}

// RecordKey identifies one record by normalized name and type.
type RecordKey struct {
	Name string
	Type RecordType
}

// Record is the stable, supported portion of a provider record.
type Record struct {
	ID    string
	Name  string
	Type  RecordType
	Value string
	TTL   int64
}

// RecordChange contains the complete desired record state.
type RecordChange struct {
	Name  string
	Type  RecordType
	Value string
	TTL   int64
}

// Provider manages supported records in existing authoritative zones.
type Provider interface {
	ListZones(context.Context) ([]Zone, error)
	TestZone(context.Context, Zone) error
	ReadRecord(context.Context, Zone, RecordKey) (Record, bool, error)
	CreateRecord(context.Context, Zone, RecordChange) (Record, error)
	UpdateRecord(context.Context, Zone, Record, RecordChange) (Record, error)
}

package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider/cloudflare"
	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider/route53"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	dnsProviderConnectionConfigKey = "dns_provider_connection"
	dnsRecordTTLSeconds            = int64(300)

	dnsProviderCloudflare = "cloudflare"
	dnsProviderRoute53    = "route53"

	dnsCredentialCloudflareToken = "cloudflare_api_token" // #nosec G101 -- credential mode label, not a credential.
	dnsCredentialAWSRuntime      = "aws_runtime"
	dnsCredentialAWSAccessKey    = "aws_access_key"
)

type dnsProviderConnectionConfig struct {
	Provider           string `json:"provider"`
	ZoneName           string `json:"zone_name"`
	ZoneID             string `json:"zone_id"`
	CredentialMode     string `json:"credential_mode"`
	CloudflareAPIToken string `json:"cloudflare_api_token,omitempty"`
	AWSAccessKeyID     string `json:"aws_access_key_id,omitempty"`
	AWSSecretAccessKey string `json:"aws_secret_access_key,omitempty"`
}

type dnsProviderFactory func(context.Context, dnsProviderConnectionConfig) (dnsprovider.Provider, error)

type desiredDNSRecord struct {
	key            dnsprovider.RecordKey
	change         dnsprovider.RecordChange
	bindAddress    string
	privateAddress bool
}

// GetDNSProviderConnection returns the active connection without credentials.
func (xs *XylonaService) GetDNSProviderConnection(_ context.Context, request *connect.Request[xylona.GetDNSProviderConnectionRequest]) (*connect.Response[xylona.GetDNSProviderConnectionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	config, configured, errConfig := xs.loadDNSProviderConnection()
	if errConfig != nil {
		return nil, dnsInternalError()
	}
	response := &xylona.GetDNSProviderConnectionResponse{Configured: configured}
	if configured {
		response.Connection = dnsProviderConnectionProto(config)
	}
	return connect.NewResponse(response), nil
}

// ListDNSProviderZones lists zones using candidate or preserved write-only credentials.
func (xs *XylonaService) ListDNSProviderZones(ctx context.Context, request *connect.Request[xylona.ListDNSProviderZonesRequest]) (*connect.Response[xylona.ListDNSProviderZonesResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	active, configured, errActive := xs.loadDNSProviderConnection()
	if errActive != nil {
		return nil, dnsInternalError()
	}
	config, errCandidate := dnsConnectionCandidate(request.Msg.GetCandidate(), active, configured, false)
	if errCandidate != nil {
		return nil, errCandidate
	}
	provider, errProvider := xs.newDNSProvider(ctx, config)
	if errProvider != nil {
		return nil, dnsProviderError(errProvider)
	}
	zones, errZones := provider.ListZones(ctx)
	if errZones != nil {
		return nil, dnsProviderError(errZones)
	}
	zoneProtos := make([]*xylona.DNSProviderZone, 0, len(zones))
	for _, zone := range zones {
		zoneProtos = append(zoneProtos, &xylona.DNSProviderZone{Id: zone.ID, Name: zone.Name})
	}
	return connect.NewResponse(&xylona.ListDNSProviderZonesResponse{Zones: zoneProtos}), nil
}

// SetDNSProviderConnection tests a candidate before replacing the encrypted active value.
func (xs *XylonaService) SetDNSProviderConnection(ctx context.Context, request *connect.Request[xylona.SetDNSProviderConnectionRequest]) (*connect.Response[xylona.SetDNSProviderConnectionResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	xs.dnsMutationMu.Lock()
	defer xs.dnsMutationMu.Unlock()

	active, configured, errActive := xs.loadDNSProviderConnection()
	if errActive != nil {
		return nil, dnsInternalError()
	}
	candidate, errCandidate := dnsConnectionCandidate(request.Msg.GetCandidate(), active, configured, true)
	if errCandidate != nil {
		return nil, errCandidate
	}
	if configured && dnsConnectionTargetChanged(active, candidate) {
		bindingCount, errCount := xs.db.CountDNSRecordBindings()
		if errCount != nil {
			return nil, dnsInternalError()
		}
		if bindingCount != 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("remove all DNS bindings before changing provider or zone"))
		}
	}

	provider, errProvider := xs.newDNSProvider(ctx, candidate)
	if errProvider != nil {
		return nil, dnsProviderError(errProvider)
	}
	errTest := provider.TestZone(ctx, dnsprovider.Zone{ID: candidate.ZoneID, Name: candidate.ZoneName})
	if errTest != nil {
		return nil, dnsProviderError(errTest)
	}
	encoded, errMarshal := json.Marshal(candidate)
	if errMarshal != nil {
		return nil, dnsInternalError()
	}
	errSave := xs.db.SetSystemConfig(dnsProviderConnectionConfigKey, string(encoded))
	if errSave != nil {
		return nil, dnsInternalError()
	}
	return connect.NewResponse(&xylona.SetDNSProviderConnectionResponse{Connection: dnsProviderConnectionProto(candidate)}), nil
}

// GetDNSBinding returns one game server's local binding and current bind-address preview.
func (xs *XylonaService) GetDNSBinding(_ context.Context, request *connect.Request[xylona.GetDNSBindingRequest]) (*connect.Response[xylona.GetDNSBindingResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	binding, errBinding := xs.db.GetDNSRecordBinding(strings.TrimSpace(request.Msg.GetGameServerId()))
	if errors.Is(errBinding, sql.ErrNoRows) {
		return connect.NewResponse(&xylona.GetDNSBindingResponse{}), nil
	}
	if errBinding != nil {
		return nil, dnsInternalError()
	}
	config, configured, errConfig := xs.loadDNSProviderConnection()
	if errConfig != nil {
		return nil, dnsInternalError()
	}
	if !configured {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("configure a DNS provider connection first"))
	}
	responseBinding, errResponse := xs.dnsBindingProto(binding, config)
	if errResponse != nil {
		return nil, errResponse
	}
	return connect.NewResponse(&xylona.GetDNSBindingResponse{Binding: responseBinding, Configured: true}), nil
}

// SetDNSBinding stores only local binding state and never calls a provider.
func (xs *XylonaService) SetDNSBinding(_ context.Context, request *connect.Request[xylona.SetDNSBindingRequest]) (*connect.Response[xylona.SetDNSBindingResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	xs.dnsMutationMu.Lock()
	defer xs.dnsMutationMu.Unlock()

	config, configured, errConfig := xs.loadDNSProviderConnection()
	if errConfig != nil {
		return nil, dnsInternalError()
	}
	if !configured {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("configure a DNS provider connection first"))
	}
	relativeName, errName := normalizeDNSRelativeName(request.Msg.GetRelativeName())
	if errName != nil {
		return nil, errName
	}
	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	_, errDesired := xs.desiredDNSRecord(gameServerID, relativeName, config)
	if errDesired != nil {
		return nil, errDesired
	}
	errSave := xs.db.UpsertDNSRecordBinding(gameServerID, relativeName)
	if errSave != nil {
		return nil, dnsInternalError()
	}
	binding, errBinding := xs.db.GetDNSRecordBinding(gameServerID)
	if errBinding != nil {
		return nil, dnsInternalError()
	}
	responseBinding, errResponse := xs.dnsBindingProto(binding, config)
	if errResponse != nil {
		return nil, errResponse
	}
	return connect.NewResponse(&xylona.SetDNSBindingResponse{Binding: responseBinding}), nil
}

// RemoveDNSBinding removes local state only.
func (xs *XylonaService) RemoveDNSBinding(_ context.Context, request *connect.Request[xylona.RemoveDNSBindingRequest]) (*connect.Response[xylona.RemoveDNSBindingResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	xs.dnsMutationMu.Lock()
	defer xs.dnsMutationMu.Unlock()

	errRemove := xs.db.RemoveDNSRecordBinding(strings.TrimSpace(request.Msg.GetGameServerId()))
	if errRemove != nil {
		return nil, dnsInternalError()
	}
	return connect.NewResponse(&xylona.RemoveDNSBindingResponse{}), nil
}

// SyncDNSBinding explicitly creates or updates the desired provider record.
func (xs *XylonaService) SyncDNSBinding(ctx context.Context, request *connect.Request[xylona.SyncDNSBindingRequest]) (*connect.Response[xylona.SyncDNSBindingResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	xs.dnsMutationMu.Lock()
	defer xs.dnsMutationMu.Unlock()

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	config, binding, desired, provider, errState := xs.loadDNSBindingActionState(ctx, gameServerID)
	if errState != nil {
		return nil, errState
	}
	zone := dnsprovider.Zone{ID: config.ZoneID, Name: config.ZoneName}
	result, errSync := xs.syncDNSRecord(ctx, provider, zone, binding, desired)
	if errSync != nil {
		return nil, errSync
	}
	updatedBinding, errBinding := xs.db.GetDNSRecordBinding(gameServerID)
	if errBinding != nil {
		return nil, dnsInternalError()
	}
	responseBinding, errResponse := xs.dnsBindingProto(updatedBinding, config)
	if errResponse != nil {
		return nil, errResponse
	}
	return connect.NewResponse(&xylona.SyncDNSBindingResponse{Binding: responseBinding, Result: result}), nil
}

// AdoptDNSBindingRecord records ownership without mutating the provider record.
func (xs *XylonaService) AdoptDNSBindingRecord(ctx context.Context, request *connect.Request[xylona.AdoptDNSBindingRecordRequest]) (*connect.Response[xylona.AdoptDNSBindingRecordResponse], error) {
	user, errUser := xs.getUserFromHeader(request.Header())
	if errUser != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	if !user.SuperUser {
		return nil, permissionDenied("superuser access required")
	}

	xs.dnsMutationMu.Lock()
	defer xs.dnsMutationMu.Unlock()

	gameServerID := strings.TrimSpace(request.Msg.GetGameServerId())
	config, binding, desired, provider, errState := xs.loadDNSBindingActionState(ctx, gameServerID)
	if errState != nil {
		return nil, errState
	}
	errTarget := xs.ensureDNSRecordTargetAvailable(binding.GameServerID, desired.key)
	if errTarget != nil {
		return nil, errTarget
	}
	record, found, errRead := provider.ReadRecord(ctx, dnsprovider.Zone{ID: config.ZoneID, Name: config.ZoneName}, desired.key)
	if errRead != nil {
		return nil, dnsProviderError(errRead)
	}
	if !found {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("DNS record does not exist"))
	}
	ownership, validOwnership := dnsRecordOwnership(record)
	if !validOwnership {
		return nil, dnsProviderError(dnsprovider.ErrUnsupported)
	}
	errSnapshot := xs.db.ReplaceDNSRecordBindingOwnership(gameServerID, ownership)
	if errors.Is(errSnapshot, db.ErrDNSRecordBindingTargetConflict) {
		return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("DNS record is already owned by another binding"))
	}
	if errSnapshot != nil {
		return nil, dnsInternalError()
	}
	updatedBinding, errBinding := xs.db.GetDNSRecordBinding(gameServerID)
	if errBinding != nil {
		return nil, dnsInternalError()
	}
	responseBinding, errResponse := xs.dnsBindingProto(updatedBinding, config)
	if errResponse != nil {
		return nil, errResponse
	}
	return connect.NewResponse(&xylona.AdoptDNSBindingRecordResponse{Binding: responseBinding}), nil
}

func (xs *XylonaService) syncDNSRecord(ctx context.Context, provider dnsprovider.Provider, zone dnsprovider.Zone, binding *db.DNSRecordBinding, desired desiredDNSRecord) (xylona.DNSSyncResult, error) {
	ownsDesired := binding.Ownership != nil && binding.Ownership.FQDN == desired.key.Name && binding.Ownership.RecordType == string(desired.key.Type)
	if ownsDesired {
		record, found, errRead := provider.ReadRecord(ctx, zone, desired.key)
		if errors.Is(errRead, dnsprovider.ErrUnsupported) {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, xs.releaseDNSOwnershipConflict(binding.GameServerID)
		}
		if errRead != nil {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(errRead)
		}
		if !found || dnsRecordDrifted(record, binding.Ownership) {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, xs.releaseDNSOwnershipConflict(binding.GameServerID)
		}
		if record.Value == desired.change.Value && record.TTL == desired.change.TTL {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNCHANGED, nil
		}
		updated, errUpdate := provider.UpdateRecord(ctx, zone, record, desired.change)
		if errUpdate != nil {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(errUpdate)
		}
		ownership, validOwnership := dnsRecordOwnership(updated)
		if !validOwnership {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(dnsprovider.ErrUnsupported)
		}
		errSnapshot := xs.db.ReplaceDNSRecordBindingOwnership(binding.GameServerID, ownership)
		if errSnapshot != nil {
			return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsSnapshotError(errSnapshot)
		}
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UPDATED, nil
	}
	errTarget := xs.ensureDNSRecordTargetAvailable(binding.GameServerID, desired.key)
	if errTarget != nil {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, errTarget
	}

	_, found, errRead := provider.ReadRecord(ctx, zone, desired.key)
	if errRead != nil {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(errRead)
	}
	if found {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, connect.NewError(connect.CodeAlreadyExists, errors.New("DNS record already exists; adopt it explicitly"))
	}
	created, errCreate := provider.CreateRecord(ctx, zone, desired.change)
	if errCreate != nil {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(errCreate)
	}
	ownership, validOwnership := dnsRecordOwnership(created)
	if !validOwnership {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsProviderError(dnsprovider.ErrUnsupported)
	}
	errSnapshot := xs.db.ReplaceDNSRecordBindingOwnership(binding.GameServerID, ownership)
	if errSnapshot != nil {
		return xylona.DNSSyncResult_DNS_SYNC_RESULT_UNSPECIFIED, dnsSnapshotError(errSnapshot)
	}
	return xylona.DNSSyncResult_DNS_SYNC_RESULT_CREATED, nil
}

func (xs *XylonaService) ensureDNSRecordTargetAvailable(gameServerID string, key dnsprovider.RecordKey) error {
	targetOwned, errTargetOwned := xs.db.DNSRecordBindingTargetOwned(gameServerID, key.Name, string(key.Type))
	if errTargetOwned != nil {
		return dnsInternalError()
	}
	if targetOwned {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("DNS record is already owned by another binding"))
	}
	return nil
}

func (xs *XylonaService) releaseDNSOwnershipConflict(gameServerID string) error {
	errClear := xs.db.ClearDNSRecordBindingOwnership(gameServerID)
	if errClear != nil {
		return dnsInternalError()
	}
	return connect.NewError(connect.CodeAborted, errors.New("DNS record changed externally; ownership was released"))
}

func (xs *XylonaService) loadDNSBindingActionState(ctx context.Context, gameServerID string) (dnsProviderConnectionConfig, *db.DNSRecordBinding, desiredDNSRecord, dnsprovider.Provider, error) {
	config, configured, errConfig := xs.loadDNSProviderConnection()
	if errConfig != nil {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, dnsInternalError()
	}
	if !configured {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("configure a DNS provider connection first"))
	}
	binding, errBinding := xs.db.GetDNSRecordBinding(gameServerID)
	if errors.Is(errBinding, sql.ErrNoRows) {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, connect.NewError(connect.CodeNotFound, errors.New("DNS binding does not exist"))
	}
	if errBinding != nil {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, dnsInternalError()
	}
	desired, errDesired := xs.desiredDNSRecord(gameServerID, binding.RelativeName, config)
	if errDesired != nil {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, errDesired
	}
	provider, errProvider := xs.newDNSProvider(ctx, config)
	if errProvider != nil {
		return dnsProviderConnectionConfig{}, nil, desiredDNSRecord{}, nil, dnsProviderError(errProvider)
	}
	return config, binding, desired, provider, nil
}

func (xs *XylonaService) desiredDNSRecord(gameServerID string, relativeName string, config dnsProviderConnectionConfig) (desiredDNSRecord, error) {
	gameServer, errGameServer := xs.db.GetGameServerByID(gameServerID)
	if errGameServer != nil {
		if errors.Is(errGameServer, sql.ErrNoRows) {
			return desiredDNSRecord{}, connect.NewError(connect.CodeNotFound, errors.New("game server does not exist"))
		}
		return desiredDNSRecord{}, dnsInternalError()
	}
	if gameServer.R.IP == nil {
		return desiredDNSRecord{}, connect.NewError(connect.CodeFailedPrecondition, errors.New("game server bind address is unavailable"))
	}
	address, errAddress := netip.ParseAddr(strings.TrimSpace(gameServer.R.IP.Address))
	if errAddress != nil {
		return desiredDNSRecord{}, connect.NewError(connect.CodeInvalidArgument, errors.New("game server bind address is invalid"))
	}
	address = address.Unmap()
	if address.IsUnspecified() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
		return desiredDNSRecord{}, connect.NewError(connect.CodeInvalidArgument, errors.New("game server bind address cannot be wildcard, loopback, or link-local"))
	}

	recordType := dnsprovider.RecordTypeAAAA
	if address.Is4() {
		recordType = dnsprovider.RecordTypeA
	}
	fqdn := relativeName + "." + normalizeDNSZoneName(config.ZoneName) + "."
	if len(fqdn) > 254 {
		return desiredDNSRecord{}, connect.NewError(connect.CodeInvalidArgument, errors.New("relative DNS name is too long for the configured zone"))
	}
	change := dnsprovider.RecordChange{Name: fqdn, Type: recordType, Value: address.String(), TTL: dnsRecordTTLSeconds}
	return desiredDNSRecord{
		key:            dnsprovider.RecordKey{Name: fqdn, Type: recordType},
		change:         change,
		bindAddress:    address.String(),
		privateAddress: address.IsPrivate(),
	}, nil
}

func (xs *XylonaService) dnsBindingProto(binding *db.DNSRecordBinding, config dnsProviderConnectionConfig) (*xylona.DNSBinding, error) {
	desired, errDesired := xs.desiredDNSRecord(binding.GameServerID, binding.RelativeName, config)
	if errDesired != nil {
		return nil, errDesired
	}
	protoBinding := &xylona.DNSBinding{
		GameServerId:       binding.GameServerID,
		RelativeName:       binding.RelativeName,
		FullyQualifiedName: desired.key.Name,
		RecordType:         dnsRecordTypeProto(desired.key.Type),
		BindAddress:        desired.bindAddress,
		PrivateAddress:     desired.privateAddress,
		TtlSeconds:         int32(dnsRecordTTLSeconds),
	}
	if binding.Ownership != nil {
		protoBinding.Owned = true
		protoBinding.OwnedFullyQualifiedName = binding.Ownership.FQDN
		protoBinding.OwnedRecordType = dnsRecordTypeProto(dnsprovider.RecordType(binding.Ownership.RecordType))
		protoBinding.OwnedValue = binding.Ownership.Value
		protoBinding.OwnedTtlSeconds = binding.Ownership.TTL
	}
	return protoBinding, nil
}

func (xs *XylonaService) loadDNSProviderConnection() (dnsProviderConnectionConfig, bool, error) {
	encoded, errGet := xs.db.GetSystemConfig(dnsProviderConnectionConfigKey)
	if errors.Is(errGet, sql.ErrNoRows) {
		return dnsProviderConnectionConfig{}, false, nil
	}
	if errGet != nil {
		return dnsProviderConnectionConfig{}, false, fmt.Errorf("load DNS provider connection: %w", errGet)
	}
	var config dnsProviderConnectionConfig
	errUnmarshal := json.Unmarshal([]byte(encoded), &config)
	if errUnmarshal != nil {
		return dnsProviderConnectionConfig{}, false, fmt.Errorf("decode DNS provider connection: %w", errUnmarshal)
	}
	return config, true, nil
}

func (xs *XylonaService) newDNSProvider(ctx context.Context, config dnsProviderConnectionConfig) (dnsprovider.Provider, error) {
	if xs.dnsProviderFactory != nil {
		return xs.dnsProviderFactory(ctx, config)
	}
	switch config.Provider {
	case dnsProviderCloudflare:
		return cloudflare.New(config.CloudflareAPIToken), nil
	case dnsProviderRoute53:
		if config.CredentialMode == dnsCredentialAWSRuntime {
			provider, errProvider := route53.New(ctx)
			if errProvider != nil {
				return nil, fmt.Errorf("create Route 53 provider: %w", errProvider)
			}
			return provider, nil
		}
		provider, errProvider := route53.NewStatic(ctx, config.AWSAccessKeyID, config.AWSSecretAccessKey)
		if errProvider != nil {
			return nil, fmt.Errorf("create Route 53 provider: %w", errProvider)
		}
		return provider, nil
	default:
		return nil, dnsprovider.ErrUnsupported
	}
}

func dnsConnectionCandidate(input *xylona.DNSProviderConnectionInput, active dnsProviderConnectionConfig, configured bool, requireZone bool) (dnsProviderConnectionConfig, error) {
	if input == nil {
		if configured && !requireZone {
			return active, nil
		}
		return dnsProviderConnectionConfig{}, connect.NewError(connect.CodeInvalidArgument, errors.New("DNS provider connection is required"))
	}
	config := dnsProviderConnectionConfig{
		Provider:           dnsProviderName(input.GetProvider()),
		ZoneName:           normalizeDNSZoneName(input.GetZoneName()),
		ZoneID:             strings.TrimSpace(input.GetZoneId()),
		CredentialMode:     dnsCredentialName(input.GetCredentialMode()),
		CloudflareAPIToken: strings.TrimSpace(input.GetCloudflareApiToken()),
		AWSAccessKeyID:     strings.TrimSpace(input.GetAwsAccessKeyId()),
		AWSSecretAccessKey: strings.TrimSpace(input.GetAwsSecretAccessKey()),
	}
	config.ZoneID = normalizeDNSZoneID(config.Provider, config.ZoneID)
	if configured && config.Provider == active.Provider && config.CredentialMode == active.CredentialMode {
		if config.CredentialMode == dnsCredentialCloudflareToken && config.CloudflareAPIToken == "" {
			config.CloudflareAPIToken = active.CloudflareAPIToken
		}
		if config.CredentialMode == dnsCredentialAWSAccessKey && config.AWSAccessKeyID == "" && config.AWSSecretAccessKey == "" {
			config.AWSAccessKeyID = active.AWSAccessKeyID
			config.AWSSecretAccessKey = active.AWSSecretAccessKey
		}
	}
	clearIrrelevantDNSCredentials(&config)
	errValidate := validateDNSConnectionConfig(config, requireZone)
	if errValidate != nil {
		return dnsProviderConnectionConfig{}, errValidate
	}
	return config, nil
}

func validateDNSConnectionConfig(config dnsProviderConnectionConfig, requireZone bool) error {
	if config.Provider == "" || config.CredentialMode == "" {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("DNS provider and credential mode are required"))
	}
	if requireZone && (config.ZoneName == "" || config.ZoneID == "") {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("DNS zone name and ID are required"))
	}
	switch config.Provider {
	case dnsProviderCloudflare:
		if config.CredentialMode != dnsCredentialCloudflareToken || config.CloudflareAPIToken == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("a Cloudflare API token is required"))
		}
	case dnsProviderRoute53:
		if config.CredentialMode == dnsCredentialAWSRuntime {
			return nil
		}
		if config.CredentialMode != dnsCredentialAWSAccessKey || config.AWSAccessKeyID == "" || config.AWSSecretAccessKey == "" {
			return connect.NewError(connect.CodeInvalidArgument, errors.New("AWS access key ID and secret access key are required"))
		}
	default:
		return connect.NewError(connect.CodeInvalidArgument, errors.New("DNS provider is unsupported"))
	}
	return nil
}

func normalizeDNSRelativeName(value string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	if name == "" || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || len(name) > 253 {
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("relative DNS name is invalid"))
	}
	for label := range strings.SplitSeq(name, ".") {
		if label == "" || label == "*" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", connect.NewError(connect.CodeInvalidArgument, errors.New("relative DNS name is invalid"))
		}
		for _, character := range label {
			valid := character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-'
			if !valid {
				return "", connect.NewError(connect.CodeInvalidArgument, errors.New("relative DNS name is invalid"))
			}
		}
	}
	return name, nil
}

func normalizeDNSZoneName(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func normalizeDNSZoneID(provider string, value string) string {
	value = strings.TrimSpace(value)
	if provider == dnsProviderRoute53 {
		return strings.TrimPrefix(value, "/hostedzone/")
	}
	return value
}

func clearIrrelevantDNSCredentials(config *dnsProviderConnectionConfig) {
	switch config.CredentialMode {
	case dnsCredentialCloudflareToken:
		config.AWSAccessKeyID = ""
		config.AWSSecretAccessKey = ""
	case dnsCredentialAWSRuntime:
		config.CloudflareAPIToken = ""
		config.AWSAccessKeyID = ""
		config.AWSSecretAccessKey = ""
	case dnsCredentialAWSAccessKey:
		config.CloudflareAPIToken = ""
	}
}

func dnsConnectionTargetChanged(left dnsProviderConnectionConfig, right dnsProviderConnectionConfig) bool {
	return left.Provider != right.Provider || normalizeDNSZoneName(left.ZoneName) != normalizeDNSZoneName(right.ZoneName) || left.ZoneID != right.ZoneID
}

func dnsProviderConnectionProto(config dnsProviderConnectionConfig) *xylona.DNSProviderConnection {
	return &xylona.DNSProviderConnection{
		Provider:              dnsProviderProto(config.Provider),
		ZoneName:              config.ZoneName,
		ZoneId:                config.ZoneID,
		CredentialMode:        dnsCredentialProto(config.CredentialMode),
		CredentialsConfigured: dnsCredentialsConfigured(config),
	}
}

func dnsCredentialsConfigured(config dnsProviderConnectionConfig) bool {
	switch config.CredentialMode {
	case dnsCredentialCloudflareToken:
		return config.CloudflareAPIToken != ""
	case dnsCredentialAWSRuntime:
		return true
	case dnsCredentialAWSAccessKey:
		return config.AWSAccessKeyID != "" && config.AWSSecretAccessKey != ""
	default:
		return false
	}
}

func dnsProviderName(provider xylona.DNSProviderKind) string {
	switch provider {
	case xylona.DNSProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE:
		return dnsProviderCloudflare
	case xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53:
		return dnsProviderRoute53
	default:
		return ""
	}
}

func dnsProviderProto(provider string) xylona.DNSProviderKind {
	if provider == dnsProviderCloudflare {
		return xylona.DNSProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE
	}
	if provider == dnsProviderRoute53 {
		return xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53
	}
	return xylona.DNSProviderKind_DNS_PROVIDER_KIND_UNSPECIFIED
}

func dnsCredentialName(mode xylona.DNSCredentialMode) string {
	switch mode {
	case xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN:
		return dnsCredentialCloudflareToken
	case xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_RUNTIME:
		return dnsCredentialAWSRuntime
	case xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY:
		return dnsCredentialAWSAccessKey
	default:
		return ""
	}
}

func dnsCredentialProto(mode string) xylona.DNSCredentialMode {
	switch mode {
	case dnsCredentialCloudflareToken:
		return xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN
	case dnsCredentialAWSRuntime:
		return xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_RUNTIME
	case dnsCredentialAWSAccessKey:
		return xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY
	default:
		return xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_UNSPECIFIED
	}
}

func dnsRecordTypeProto(recordType dnsprovider.RecordType) xylona.DNSRecordType {
	if recordType == dnsprovider.RecordTypeA {
		return xylona.DNSRecordType_DNS_RECORD_TYPE_A
	}
	if recordType == dnsprovider.RecordTypeAAAA {
		return xylona.DNSRecordType_DNS_RECORD_TYPE_AAAA
	}
	return xylona.DNSRecordType_DNS_RECORD_TYPE_UNSPECIFIED
}

func dnsRecordOwnership(record dnsprovider.Record) (db.DNSRecordOwnership, bool) {
	if record.TTL < 0 || record.TTL > math.MaxInt32 {
		return db.DNSRecordOwnership{}, false
	}
	var providerRecordID *string
	if record.ID != "" {
		providerRecordID = new(record.ID)
	}
	return db.DNSRecordOwnership{
		ProviderRecordID: providerRecordID,
		FQDN:             record.Name,
		RecordType:       string(record.Type),
		Value:            record.Value,
		TTL:              int32(record.TTL), // #nosec G115 -- range checked above.
	}, true
}

func dnsRecordDrifted(record dnsprovider.Record, ownership *db.DNSRecordOwnership) bool {
	if ownership.ProviderRecordID != nil && record.ID != *ownership.ProviderRecordID {
		return true
	}
	return record.Name != ownership.FQDN || string(record.Type) != ownership.RecordType || record.Value != ownership.Value || record.TTL != int64(ownership.TTL)
}

func dnsSnapshotError(err error) error {
	if errors.Is(err, db.ErrDNSRecordBindingTargetConflict) {
		return connect.NewError(connect.CodeAlreadyExists, errors.New("DNS record is already owned by another binding"))
	}
	return dnsInternalError()
}

func dnsProviderError(err error) error {
	switch {
	case errors.Is(err, dnsprovider.ErrUnauthorized):
		return connect.NewError(connect.CodeUnauthenticated, errors.New("DNS provider credentials were rejected"))
	case errors.Is(err, dnsprovider.ErrForbidden):
		return connect.NewError(connect.CodePermissionDenied, errors.New("DNS provider access was denied"))
	case errors.Is(err, dnsprovider.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, errors.New("DNS provider resource was not found"))
	case errors.Is(err, dnsprovider.ErrConflict):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("DNS provider reported a conflict"))
	case errors.Is(err, dnsprovider.ErrUnsupported):
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("DNS record shape is unsupported"))
	default:
		return connect.NewError(connect.CodeUnavailable, errors.New("DNS provider is unavailable"))
	}
}

func dnsInternalError() error {
	return connect.NewError(connect.CodeInternal, errors.New("DNS operation failed"))
}

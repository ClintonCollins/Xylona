package rpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

type fakeDNSProvider struct {
	zones       []dnsprovider.Zone
	records     map[dnsprovider.RecordKey]dnsprovider.Record
	testZoneErr error
	listErr     error
	readErr     error
	createErr   error
	updateErr   error
	testCalls   int
	listCalls   int
	readCalls   int
	createCalls int
	updateCalls int
	readEntered chan struct{}
	readRelease <-chan struct{}
	readOnce    sync.Once
}

func newFakeDNSProvider() *fakeDNSProvider {
	return &fakeDNSProvider{
		zones:   []dnsprovider.Zone{{ID: "zone-1", Name: "example.com."}},
		records: make(map[dnsprovider.RecordKey]dnsprovider.Record),
	}
}

func (provider *fakeDNSProvider) ListZones(context.Context) ([]dnsprovider.Zone, error) {
	provider.listCalls++
	return provider.zones, provider.listErr
}

func (provider *fakeDNSProvider) TestZone(context.Context, dnsprovider.Zone) error {
	provider.testCalls++
	return provider.testZoneErr
}

func (provider *fakeDNSProvider) ReadRecord(_ context.Context, _ dnsprovider.Zone, key dnsprovider.RecordKey) (dnsprovider.Record, bool, error) {
	provider.readCalls++
	if provider.readEntered != nil {
		provider.readOnce.Do(func() { close(provider.readEntered) })
		<-provider.readRelease
	}
	if provider.readErr != nil {
		return dnsprovider.Record{}, false, provider.readErr
	}
	record, found := provider.records[key]
	return record, found, nil
}

func (provider *fakeDNSProvider) CreateRecord(_ context.Context, _ dnsprovider.Zone, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	provider.createCalls++
	if provider.createErr != nil {
		return dnsprovider.Record{}, provider.createErr
	}
	record := dnsprovider.Record{ID: fmt.Sprintf("record-%d", provider.createCalls), Name: change.Name, Type: change.Type, Value: change.Value, TTL: change.TTL}
	provider.records[dnsprovider.RecordKey{Name: change.Name, Type: change.Type}] = record
	return record, nil
}

func (provider *fakeDNSProvider) UpdateRecord(_ context.Context, _ dnsprovider.Zone, record dnsprovider.Record, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	provider.updateCalls++
	if provider.updateErr != nil {
		return dnsprovider.Record{}, provider.updateErr
	}
	updated := dnsprovider.Record{ID: record.ID, Name: change.Name, Type: change.Type, Value: change.Value, TTL: change.TTL}
	provider.records[dnsprovider.RecordKey{Name: change.Name, Type: change.Type}] = updated
	return updated, nil
}

func TestDNSRPCRequiresSuperuser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	tests := []struct {
		name string
		call func() error
	}{
		{"get connection", func() error {
			request := connect.NewRequest(&xylona.GetDNSProviderConnectionRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.GetDNSProviderConnection(t.Context(), request)
			return err
		}},
		{"list zones", func() error {
			request := connect.NewRequest(&xylona.ListDNSProviderZonesRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.ListDNSProviderZones(t.Context(), request)
			return err
		}},
		{"set connection", func() error {
			request := connect.NewRequest(&xylona.SetDNSProviderConnectionRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.SetDNSProviderConnection(t.Context(), request)
			return err
		}},
		{"get binding", func() error {
			request := connect.NewRequest(&xylona.GetDNSBindingRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.GetDNSBinding(t.Context(), request)
			return err
		}},
		{"set binding", func() error {
			request := connect.NewRequest(&xylona.SetDNSBindingRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.SetDNSBinding(t.Context(), request)
			return err
		}},
		{"remove binding", func() error {
			request := connect.NewRequest(&xylona.RemoveDNSBindingRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.RemoveDNSBinding(t.Context(), request)
			return err
		}},
		{"sync binding", func() error {
			request := connect.NewRequest(&xylona.SyncDNSBindingRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.SyncDNSBinding(t.Context(), request)
			return err
		}},
		{"adopt binding", func() error {
			request := connect.NewRequest(&xylona.AdoptDNSBindingRecordRequest{})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")
			_, err := fixture.service.AdoptDNSBindingRecord(t.Context(), request)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if connect.CodeOf(err) != connect.CodePermissionDenied {
				t.Fatalf("code = %v, want %v", connect.CodeOf(err), connect.CodePermissionDenied)
			}
		})
	}
}

func TestDNSProviderConnectionLifecycle(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	provider := newFakeDNSProvider()
	var factoryConfigs []dnsProviderConnectionConfig
	fixture.service.dnsProviderFactory = func(_ context.Context, config dnsProviderConnectionConfig) (dnsprovider.Provider, error) {
		factoryConfigs = append(factoryConfigs, config)
		return provider, nil
	}

	setRequest := connect.NewRequest(&xylona.SetDNSProviderConnectionRequest{Candidate: cloudflareCandidate("first-secret")})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setRequest, "user-admin")
	setResponse, errSet := fixture.service.SetDNSProviderConnection(t.Context(), setRequest)
	if errSet != nil {
		t.Fatalf("SetDNSProviderConnection() error = %v", errSet)
	}
	if !setResponse.Msg.GetConnection().GetCredentialsConfigured() || provider.testCalls != 1 {
		t.Fatalf("activation response = %+v, test calls = %d", setResponse.Msg.GetConnection(), provider.testCalls)
	}

	getRequest := connect.NewRequest(&xylona.GetDNSProviderConnectionRequest{})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-admin")
	getResponse, errGet := fixture.service.GetDNSProviderConnection(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetDNSProviderConnection() error = %v", errGet)
	}
	if !getResponse.Msg.GetConfigured() || getResponse.Msg.GetConnection().GetZoneId() != "zone-1" {
		t.Fatalf("GetDNSProviderConnection() = %+v", getResponse.Msg)
	}

	listRequest := connect.NewRequest(&xylona.ListDNSProviderZonesRequest{Candidate: cloudflareCandidate("")})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, listRequest, "user-admin")
	listResponse, errList := fixture.service.ListDNSProviderZones(t.Context(), listRequest)
	if errList != nil {
		t.Fatalf("ListDNSProviderZones() error = %v", errList)
	}
	if len(listResponse.Msg.GetZones()) != 1 || factoryConfigs[len(factoryConfigs)-1].CloudflareAPIToken != "first-secret" {
		t.Fatalf("ListDNSProviderZones() zones = %+v, config = %+v", listResponse.Msg.GetZones(), factoryConfigs[len(factoryConfigs)-1])
	}
	preservedRequest := connect.NewRequest(&xylona.SetDNSProviderConnectionRequest{Candidate: cloudflareCandidate("")})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, preservedRequest, "user-admin")
	_, errPreserved := fixture.service.SetDNSProviderConnection(t.Context(), preservedRequest)
	if errPreserved != nil || factoryConfigs[len(factoryConfigs)-1].CloudflareAPIToken != "first-secret" {
		t.Fatalf("SetDNSProviderConnection() with preserved token error = %v, config = %+v", errPreserved, factoryConfigs[len(factoryConfigs)-1])
	}

	storedBefore, errStoredBefore := fixture.conn.GetSystemConfig(dnsProviderConnectionConfigKey)
	if errStoredBefore != nil {
		t.Fatalf("GetSystemConfig() before failed activation error = %v", errStoredBefore)
	}
	provider.testZoneErr = errors.New("provider detail containing second-secret")
	failedRequest := connect.NewRequest(&xylona.SetDNSProviderConnectionRequest{Candidate: cloudflareCandidate("second-secret")})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, failedRequest, "user-admin")
	_, errFailed := fixture.service.SetDNSProviderConnection(t.Context(), failedRequest)
	if connect.CodeOf(errFailed) != connect.CodeUnavailable || strings.Contains(errFailed.Error(), "second-secret") || strings.Contains(errFailed.Error(), "provider detail") {
		t.Fatalf("failed activation error = %v", errFailed)
	}
	storedAfter, errStoredAfter := fixture.conn.GetSystemConfig(dnsProviderConnectionConfigKey)
	if errStoredAfter != nil {
		t.Fatalf("GetSystemConfig() after failed activation error = %v", errStoredAfter)
	}
	if storedAfter != storedBefore {
		t.Fatal("failed activation changed active configuration")
	}

	provider.testZoneErr = nil
	setServerIP(t, fixture, "203.0.113.10")
	setBinding(t, fixture, "game")
	testsBefore := provider.testCalls
	replacement := connect.NewRequest(&xylona.SetDNSProviderConnectionRequest{Candidate: &xylona.DNSProviderConnectionInput{
		Provider:       xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53,
		ZoneName:       "other.example",
		ZoneId:         "zone-2",
		CredentialMode: xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_RUNTIME,
	}})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, replacement, "user-admin")
	_, errReplacement := fixture.service.SetDNSProviderConnection(t.Context(), replacement)
	if connect.CodeOf(errReplacement) != connect.CodeFailedPrecondition || provider.testCalls != testsBefore {
		t.Fatalf("replacement error = %v, test calls = %d, want %d", errReplacement, provider.testCalls, testsBefore)
	}
}

func TestDNSConnectionCandidateCredentialHandling(t *testing.T) {
	active := dnsProviderConnectionConfig{
		Provider:           dnsProviderRoute53,
		CredentialMode:     dnsCredentialAWSAccessKey,
		AWSAccessKeyID:     "stored-id",
		AWSSecretAccessKey: "stored-secret",
	}
	tests := []struct {
		name       string
		input      *xylona.DNSProviderConnectionInput
		configured bool
		want       dnsProviderConnectionConfig
		wantCode   connect.Code
	}{
		{
			name: "Cloudflare drops AWS fields",
			input: &xylona.DNSProviderConnectionInput{
				Provider:           xylona.DNSProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
				CredentialMode:     xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN,
				CloudflareApiToken: "token",
				AwsAccessKeyId:     "unused-id",
				AwsSecretAccessKey: "unused-secret",
			},
			want: dnsProviderConnectionConfig{Provider: dnsProviderCloudflare, CredentialMode: dnsCredentialCloudflareToken, CloudflareAPIToken: "token"},
		},
		{
			name: "runtime chain drops explicit fields",
			input: &xylona.DNSProviderConnectionInput{
				Provider:           xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53,
				CredentialMode:     xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_RUNTIME,
				CloudflareApiToken: "unused-token",
				AwsAccessKeyId:     "unused-id",
				AwsSecretAccessKey: "unused-secret",
			},
			want: dnsProviderConnectionConfig{Provider: dnsProviderRoute53, CredentialMode: dnsCredentialAWSRuntime},
		},
		{
			name: "blank static pair preserves both",
			input: &xylona.DNSProviderConnectionInput{
				Provider:       xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53,
				CredentialMode: xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY,
			},
			configured: true,
			want:       active,
		},
		{
			name: "partial static pair is rejected",
			input: &xylona.DNSProviderConnectionInput{
				Provider:       xylona.DNSProviderKind_DNS_PROVIDER_KIND_ROUTE53,
				CredentialMode: xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY,
				AwsAccessKeyId: "new-id",
			},
			configured: true,
			wantCode:   connect.CodeInvalidArgument,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, errCandidate := dnsConnectionCandidate(test.input, active, test.configured, false)
			if test.wantCode != 0 {
				if connect.CodeOf(errCandidate) != test.wantCode {
					t.Fatalf("code = %v, want %v", connect.CodeOf(errCandidate), test.wantCode)
				}
				return
			}
			if errCandidate != nil {
				t.Fatalf("dnsConnectionCandidate() error = %v", errCandidate)
			}
			if got != test.want {
				t.Fatalf("dnsConnectionCandidate() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestDNSBindingAddressValidationAndLocalSave(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	storeDNSConnection(t, fixture)
	provider := newFakeDNSProvider()
	fixture.service.dnsProviderFactory = func(context.Context, dnsProviderConnectionConfig) (dnsprovider.Provider, error) {
		return provider, nil
	}

	tests := []struct {
		name        string
		address     string
		wantCode    connect.Code
		wantType    xylona.DNSRecordType
		wantAddress string
		wantPrivate bool
	}{
		{"wildcard IPv4", "0.0.0.0", connect.CodeInvalidArgument, 0, "", false},
		{"loopback IPv4", "127.0.0.1", connect.CodeInvalidArgument, 0, "", false},
		{"link-local IPv4", "169.254.1.2", connect.CodeInvalidArgument, 0, "", false},
		{"wildcard IPv6", "::", connect.CodeInvalidArgument, 0, "", false},
		{"loopback IPv6", "::1", connect.CodeInvalidArgument, 0, "", false},
		{"link-local IPv6", "fe80::1", connect.CodeInvalidArgument, 0, "", false},
		{"private IPv4", "10.0.0.8", 0, xylona.DNSRecordType_DNS_RECORD_TYPE_A, "10.0.0.8", true},
		{"mapped IPv4", "::ffff:203.0.113.8", 0, xylona.DNSRecordType_DNS_RECORD_TYPE_A, "203.0.113.8", false},
		{"IPv6", "2001:db8::8", 0, xylona.DNSRecordType_DNS_RECORD_TYPE_AAAA, "2001:db8::8", false},
		{"multicast is not rejected by binding policy", "239.1.1.8", 0, xylona.DNSRecordType_DNS_RECORD_TYPE_A, "239.1.1.8", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setServerIP(t, fixture, test.address)
			request := connect.NewRequest(&xylona.SetDNSBindingRequest{GameServerId: "server-local-1", RelativeName: "Game.EU"})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
			response, errSet := fixture.service.SetDNSBinding(t.Context(), request)
			if test.wantCode != 0 {
				if connect.CodeOf(errSet) != test.wantCode {
					t.Fatalf("code = %v, want %v", connect.CodeOf(errSet), test.wantCode)
				}
				return
			}
			if errSet != nil {
				t.Fatalf("SetDNSBinding() error = %v", errSet)
			}
			binding := response.Msg.GetBinding()
			if binding.GetRelativeName() != "game.eu" || binding.GetRecordType() != test.wantType || binding.GetBindAddress() != test.wantAddress || binding.GetPrivateAddress() != test.wantPrivate || binding.GetTtlSeconds() != 300 {
				t.Fatalf("SetDNSBinding() binding = %+v", binding)
			}
		})
	}
	if provider.readCalls != 0 || provider.createCalls != 0 || provider.updateCalls != 0 || provider.testCalls != 0 || provider.listCalls != 0 {
		t.Fatalf("local saves called provider: %+v", provider)
	}

	wildcardRequest := connect.NewRequest(&xylona.SetDNSBindingRequest{GameServerId: "server-local-1", RelativeName: "*.game"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, wildcardRequest, "user-admin")
	_, errWildcard := fixture.service.SetDNSBinding(t.Context(), wildcardRequest)
	if connect.CodeOf(errWildcard) != connect.CodeInvalidArgument {
		t.Fatalf("wildcard relative name code = %v", connect.CodeOf(errWildcard))
	}
}

func TestDNSBindingManualOwnershipStateMachine(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	storeDNSConnection(t, fixture)
	provider := newFakeDNSProvider()
	fixture.service.dnsProviderFactory = func(context.Context, dnsProviderConnectionConfig) (dnsprovider.Provider, error) {
		return provider, nil
	}
	setServerIP(t, fixture, "203.0.113.10")
	setBinding(t, fixture, "game")

	responseCreate := syncBinding(t, fixture)
	if responseCreate.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_CREATED || !responseCreate.Msg.GetBinding().GetOwned() || provider.createCalls != 1 {
		t.Fatalf("create response = %+v, create calls = %d", responseCreate.Msg, provider.createCalls)
	}
	responseNoop := syncBinding(t, fixture)
	if responseNoop.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_UNCHANGED || provider.updateCalls != 0 {
		t.Fatalf("no-op response = %+v, update calls = %d", responseNoop.Msg, provider.updateCalls)
	}

	setServerIP(t, fixture, "203.0.113.11")
	responseUpdate := syncBinding(t, fixture)
	if responseUpdate.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_UPDATED || provider.updateCalls != 1 {
		t.Fatalf("update response = %+v, update calls = %d", responseUpdate.Msg, provider.updateCalls)
	}

	keyA := dnsprovider.RecordKey{Name: "game.example.com.", Type: dnsprovider.RecordTypeA}
	drifted := provider.records[keyA]
	drifted.Value = "203.0.113.99"
	drifted.TTL = 0
	provider.records[keyA] = drifted
	errDrift := syncBindingError(t, fixture)
	if connect.CodeOf(errDrift) != connect.CodeAborted {
		t.Fatalf("drift sync error = %v", errDrift)
	}
	bindingAfterDrift, errBinding := fixture.conn.GetDNSRecordBinding("server-local-1")
	if errBinding != nil || bindingAfterDrift.Ownership != nil {
		t.Fatalf("binding after drift = %+v, error = %v", bindingAfterDrift, errBinding)
	}

	errExisting := syncBindingError(t, fixture)
	if connect.CodeOf(errExisting) != connect.CodeAlreadyExists || provider.updateCalls != 1 {
		t.Fatalf("existing unowned sync error = %v, update calls = %d", errExisting, provider.updateCalls)
	}
	adoptRequest := connect.NewRequest(&xylona.AdoptDNSBindingRecordRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, adoptRequest, "user-admin")
	adoptResponse, errAdopt := fixture.service.AdoptDNSBindingRecord(t.Context(), adoptRequest)
	if errAdopt != nil || !adoptResponse.Msg.GetBinding().GetOwned() || provider.createCalls != 1 || provider.updateCalls != 1 {
		t.Fatalf("adopt response = %+v, error = %v, provider = %+v", adoptResponse, errAdopt, provider)
	}
	responseAfterAdopt := syncBinding(t, fixture)
	if responseAfterAdopt.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_UPDATED || provider.updateCalls != 2 {
		t.Fatalf("sync after adoption = %+v, update calls = %d", responseAfterAdopt.Msg, provider.updateCalls)
	}

	setBinding(t, fixture, "new")
	oldRecord := provider.records[keyA]
	responseNameChange := syncBinding(t, fixture)
	if responseNameChange.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_CREATED || provider.records[keyA] != oldRecord {
		t.Fatalf("name change response = %+v, old record = %+v", responseNameChange.Msg, provider.records[keyA])
	}

	setServerIP(t, fixture, "2001:db8::11")
	responseFamilyChange := syncBinding(t, fixture)
	if responseFamilyChange.Msg.GetResult() != xylona.DNSSyncResult_DNS_SYNC_RESULT_CREATED || responseFamilyChange.Msg.GetBinding().GetRecordType() != xylona.DNSRecordType_DNS_RECORD_TYPE_AAAA {
		t.Fatalf("family change response = %+v", responseFamilyChange.Msg)
	}
	keyNewA := dnsprovider.RecordKey{Name: "new.example.com.", Type: dnsprovider.RecordTypeA}
	_, oldAExists := provider.records[keyNewA]
	if !oldAExists {
		t.Fatal("family change removed the old A record")
	}

	provider.readErr = errors.New("raw provider secret detail")
	errProvider := syncBindingError(t, fixture)
	if connect.CodeOf(errProvider) != connect.CodeUnavailable || strings.Contains(errProvider.Error(), "raw provider") {
		t.Fatalf("provider error was not sanitized: %v", errProvider)
	}
	bindingAfterProviderError, errBindingAfterProviderError := fixture.conn.GetDNSRecordBinding("server-local-1")
	if errBindingAfterProviderError != nil || bindingAfterProviderError.Ownership == nil {
		t.Fatalf("provider failure changed ownership: binding = %+v, error = %v", bindingAfterProviderError, errBindingAfterProviderError)
	}
	provider.readErr = nil
	writeCalls := provider.createCalls + provider.updateCalls
	removeRequest := connect.NewRequest(&xylona.RemoveDNSBindingRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, removeRequest, "user-admin")
	_, errRemove := fixture.service.RemoveDNSBinding(t.Context(), removeRequest)
	if errRemove != nil {
		t.Fatalf("RemoveDNSBinding() error = %v", errRemove)
	}
	_, errMissing := fixture.conn.GetDNSRecordBinding("server-local-1")
	if !errors.Is(errMissing, sql.ErrNoRows) || provider.createCalls+provider.updateCalls != writeCalls {
		t.Fatalf("remove local state error = %v, provider writes = %d, want %d", errMissing, provider.createCalls+provider.updateCalls, writeCalls)
	}
}

func TestDNSBindingRejectsLocallyOwnedTargetBeforeProviderMutation(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	storeDNSConnection(t, fixture)
	provider := newFakeDNSProvider()
	fixture.service.dnsProviderFactory = func(context.Context, dnsProviderConnectionConfig) (dnsprovider.Provider, error) {
		return provider, nil
	}
	setServerIP(t, fixture, "203.0.113.10")
	setBinding(t, fixture, "game")
	insertTestGameServer(t, fixture, "server-owner-2")
	errBinding := fixture.conn.UpsertDNSRecordBinding("server-owner-2", "game")
	if errBinding != nil {
		t.Fatalf("UpsertDNSRecordBinding(owner) error = %v", errBinding)
	}
	errOwnership := fixture.conn.ReplaceDNSRecordBindingOwnership("server-owner-2", db.DNSRecordOwnership{
		FQDN:       "game.example.com.",
		RecordType: string(dnsprovider.RecordTypeA),
		Value:      "203.0.113.9",
		TTL:        300,
	})
	if errOwnership != nil {
		t.Fatalf("ReplaceDNSRecordBindingOwnership(owner) error = %v", errOwnership)
	}

	errSync := syncBindingError(t, fixture)
	if connect.CodeOf(errSync) != connect.CodeFailedPrecondition {
		t.Fatalf("SyncDNSBinding() error = %v, want failed precondition", errSync)
	}
	adoptRequest := connect.NewRequest(&xylona.AdoptDNSBindingRecordRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, adoptRequest, "user-admin")
	_, errAdopt := fixture.service.AdoptDNSBindingRecord(t.Context(), adoptRequest)
	if connect.CodeOf(errAdopt) != connect.CodeFailedPrecondition {
		t.Fatalf("AdoptDNSBindingRecord() error = %v, want failed precondition", errAdopt)
	}
	if provider.readCalls != 0 || provider.createCalls != 0 || provider.updateCalls != 0 {
		t.Fatalf("provider calls after local ownership conflict = read %d, create %d, update %d", provider.readCalls, provider.createCalls, provider.updateCalls)
	}
}

func cloudflareCandidate(token string) *xylona.DNSProviderConnectionInput {
	return &xylona.DNSProviderConnectionInput{
		Provider:           xylona.DNSProviderKind_DNS_PROVIDER_KIND_CLOUDFLARE,
		ZoneName:           "Example.COM.",
		ZoneId:             "zone-1",
		CredentialMode:     xylona.DNSCredentialMode_DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN,
		CloudflareApiToken: token,
	}
}

func storeDNSConnection(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()
	errSet := fixture.conn.SetSystemConfig(dnsProviderConnectionConfigKey, `{"provider":"cloudflare","zone_name":"example.com","zone_id":"zone-1","credential_mode":"cloudflare_api_token","cloudflare_api_token":"stored-secret"}`)
	if errSet != nil {
		t.Fatalf("SetSystemConfig() error = %v", errSet)
	}
}

func setServerIP(t *testing.T, fixture *rbacRPCFixture, address string) {
	t.Helper()
	_, errIP := fixture.conn.SQLDb.ExecContext(t.Context(), `insert into ip (address, usable, external, node_id) values (?, ?, ?, ?) on conflict(address, node_id) do nothing`, address, true, true, "node-local")
	if errIP != nil {
		t.Fatalf("insert IP %q: %v", address, errIP)
	}
	_, errServer := fixture.conn.SQLDb.ExecContext(t.Context(), `update game_server set ip = ? where id = ?`, address, "server-local-1")
	if errServer != nil {
		t.Fatalf("update game server IP %q: %v", address, errServer)
	}
}

func setBinding(t *testing.T, fixture *rbacRPCFixture, relativeName string) *connect.Response[xylona.SetDNSBindingResponse] {
	t.Helper()
	request := connect.NewRequest(&xylona.SetDNSBindingRequest{GameServerId: "server-local-1", RelativeName: relativeName})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	response, errSet := fixture.service.SetDNSBinding(t.Context(), request)
	if errSet != nil {
		t.Fatalf("SetDNSBinding() error = %v", errSet)
	}
	return response
}

func syncBinding(t *testing.T, fixture *rbacRPCFixture) *connect.Response[xylona.SyncDNSBindingResponse] {
	t.Helper()
	request := connect.NewRequest(&xylona.SyncDNSBindingRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	response, errSync := fixture.service.SyncDNSBinding(t.Context(), request)
	if errSync != nil {
		t.Fatalf("SyncDNSBinding() error = %v", errSync)
	}
	return response
}

func syncBindingError(t *testing.T, fixture *rbacRPCFixture) error {
	t.Helper()
	request := connect.NewRequest(&xylona.SyncDNSBindingRequest{GameServerId: "server-local-1"})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	_, errSync := fixture.service.SyncDNSBinding(t.Context(), request)
	return errSync
}

var _ dnsprovider.Provider = (*fakeDNSProvider)(nil)

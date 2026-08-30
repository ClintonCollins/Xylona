// Package cloudflare implements DNS record management through Cloudflare's API.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
)

const apiURL = "https://api.cloudflare.com/client/v4"

// Client manages DNS records with a scoped API token.
type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type cloudflareZone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type cloudflareRecord struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
	Proxied *bool  `json:"proxied"`
}

type responseEnvelope[T any] struct {
	Success    bool `json:"success"`
	Result     T    `json:"result"`
	ResultInfo struct {
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

type recordPayload struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int64  `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// New returns a Cloudflare DNS client using token.
func New(token string) *Client {
	return &Client{
		token:   token,
		baseURL: apiURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ListZones lists active zones visible to the token.
func (client *Client) ListZones(ctx context.Context) ([]dnsprovider.Zone, error) {
	var zones []dnsprovider.Zone
	for page := 1; ; page++ {
		query := url.Values{
			"page":     {strconv.Itoa(page)},
			"per_page": {"50"},
			"status":   {"active"},
		}
		var response responseEnvelope[[]cloudflareZone]
		err := client.do(ctx, http.MethodGet, "/zones", query, nil, &response)
		if err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, dnsprovider.ErrUnavailable
		}
		for _, zone := range response.Result {
			if zone.ID == "" || zone.Name == "" {
				return nil, dnsprovider.ErrUnavailable
			}
			zones = append(zones, dnsprovider.Zone{ID: zone.ID, Name: normalizeName(zone.Name)})
		}
		if response.ResultInfo.TotalPages <= page {
			return zones, nil
		}
	}
}

// TestZone verifies access to the exact zone.
func (client *Client) TestZone(ctx context.Context, zone dnsprovider.Zone) error {
	if zone.ID == "" || zone.Name == "" {
		return dnsprovider.ErrNotFound
	}

	var response responseEnvelope[cloudflareZone]
	err := client.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zone.ID), nil, nil, &response)
	if err != nil {
		return err
	}
	if !response.Success {
		return dnsprovider.ErrUnavailable
	}
	if response.Result.ID != zone.ID || normalizeName(response.Result.Name) != normalizeName(zone.Name) {
		return dnsprovider.ErrNotFound
	}
	if response.Result.Status != "active" {
		return dnsprovider.ErrConflict
	}
	return nil
}

// ReadRecord reads one supported DNS-only record.
func (client *Client) ReadRecord(ctx context.Context, zone dnsprovider.Zone, key dnsprovider.RecordKey) (dnsprovider.Record, bool, error) {
	if zone.ID == "" || !validRecordType(key.Type) || normalizeName(key.Name) == "." {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnsupported
	}

	query := url.Values{
		"name":     {normalizeName(key.Name)},
		"per_page": {"100"},
		"type":     {string(key.Type)},
	}
	var response responseEnvelope[[]cloudflareRecord]
	err := client.do(ctx, http.MethodGet, "/zones/"+url.PathEscape(zone.ID)+"/dns_records", query, nil, &response)
	if err != nil {
		return dnsprovider.Record{}, false, err
	}
	if !response.Success {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnavailable
	}
	if len(response.Result) == 0 {
		return dnsprovider.Record{}, false, nil
	}
	if len(response.Result) != 1 {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnsupported
	}

	record, err := supportedRecord(response.Result[0])
	if err != nil {
		return dnsprovider.Record{}, false, err
	}
	if record.Name != normalizeName(key.Name) || record.Type != key.Type {
		return dnsprovider.Record{}, false, dnsprovider.ErrUnsupported
	}
	return record, true, nil
}

// CreateRecord creates one DNS-only record.
func (client *Client) CreateRecord(ctx context.Context, zone dnsprovider.Zone, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	return client.writeRecord(ctx, http.MethodPost, "/zones/"+url.PathEscape(zone.ID)+"/dns_records", zone, change)
}

// UpdateRecord replaces one owned DNS-only record.
func (client *Client) UpdateRecord(ctx context.Context, zone dnsprovider.Zone, record dnsprovider.Record, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	if record.ID == "" {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	path := "/zones/" + url.PathEscape(zone.ID) + "/dns_records/" + url.PathEscape(record.ID)
	return client.writeRecord(ctx, http.MethodPut, path, zone, change)
}

func (client *Client) writeRecord(ctx context.Context, method string, path string, zone dnsprovider.Zone, change dnsprovider.RecordChange) (dnsprovider.Record, error) {
	if zone.ID == "" || !validRecordType(change.Type) || normalizeName(change.Name) == "." || change.Value == "" || change.TTL < 1 {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	payload := recordPayload{
		Type:    string(change.Type),
		Name:    normalizeName(change.Name),
		Content: change.Value,
		TTL:     change.TTL,
		Proxied: false,
	}
	var response responseEnvelope[cloudflareRecord]
	err := client.do(ctx, method, path, nil, payload, &response)
	if err != nil {
		return dnsprovider.Record{}, err
	}
	if !response.Success {
		return dnsprovider.Record{}, dnsprovider.ErrUnavailable
	}
	record, err := supportedRecord(response.Result)
	if err != nil {
		return dnsprovider.Record{}, err
	}
	if record.Name != payload.Name || record.Type != change.Type || record.Value != change.Value || record.TTL != change.TTL {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	return record, nil
}

func (client *Client) do(ctx context.Context, method string, path string, query url.Values, payload any, output any) (err error) {
	if client == nil || client.token == "" || client.baseURL == "" || client.httpClient == nil {
		return dnsprovider.ErrUnauthorized
	}

	var body io.Reader
	if payload != nil {
		data, errMarshal := json.Marshal(payload)
		if errMarshal != nil {
			return dnsprovider.ErrUnavailable
		}
		body = bytes.NewReader(data)
	}
	request, errRequest := http.NewRequestWithContext(ctx, method, client.baseURL+path, body)
	if errRequest != nil {
		return dnsprovider.ErrUnavailable
	}
	request.URL.RawQuery = query.Encode()
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, errDo := client.httpClient.Do(request)
	if errDo != nil {
		return dnsprovider.ErrUnavailable
	}
	defer func() {
		errClose := response.Body.Close()
		if err == nil && errClose != nil {
			err = dnsprovider.ErrUnavailable
		}
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return statusError(response.StatusCode)
	}
	errDecode := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output)
	if errDecode != nil {
		return dnsprovider.ErrUnavailable
	}
	return nil
}

func supportedRecord(providerRecord cloudflareRecord) (dnsprovider.Record, error) {
	recordType := dnsprovider.RecordType(providerRecord.Type)
	if providerRecord.ID == "" || !validRecordType(recordType) || providerRecord.Name == "" || providerRecord.Content == "" || providerRecord.TTL < 1 || providerRecord.Proxied == nil || *providerRecord.Proxied {
		return dnsprovider.Record{}, dnsprovider.ErrUnsupported
	}
	return dnsprovider.Record{
		ID:    providerRecord.ID,
		Name:  normalizeName(providerRecord.Name),
		Type:  recordType,
		Value: providerRecord.Content,
		TTL:   providerRecord.TTL,
	}, nil
}

func statusError(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return dnsprovider.ErrUnauthorized
	case http.StatusForbidden:
		return dnsprovider.ErrForbidden
	case http.StatusNotFound:
		return dnsprovider.ErrNotFound
	case http.StatusConflict:
		return dnsprovider.ErrConflict
	default:
		return dnsprovider.ErrUnavailable
	}
}

func validRecordType(recordType dnsprovider.RecordType) bool {
	return recordType == dnsprovider.RecordTypeA || recordType == dnsprovider.RecordTypeAAAA
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), ".")) + "."
}

var _ dnsprovider.Provider = (*Client)(nil)

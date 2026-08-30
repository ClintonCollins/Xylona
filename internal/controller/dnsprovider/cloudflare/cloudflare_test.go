package cloudflare

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/controller/dnsprovider"
)

func TestClient(t *testing.T) {
	t.Run("lists and tests zones with bearer token", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Authorization") != "Bearer scoped-token" {
				t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
			}

			switch request.URL.Path {
			case "/zones":
				writeResponse(t, response, `{"success":true,"result":[{"id":"zone-id","name":"Example.COM"}],"result_info":{"total_pages":1}}`)
			case "/zones/zone-id":
				writeResponse(t, response, `{"success":true,"result":{"id":"zone-id","name":"example.com","status":"active"}}`)
			default:
				http.NotFound(response, request)
			}
		}))
		defer server.Close()

		client := newTestClient(server, "scoped-token")
		zones, err := client.ListZones(t.Context())
		if err != nil {
			t.Fatalf("ListZones() error = %v", err)
		}
		if len(zones) != 1 || zones[0].ID != "zone-id" || zones[0].Name != "example.com." {
			t.Fatalf("ListZones() = %#v", zones)
		}

		err = client.TestZone(t.Context(), dnsprovider.Zone{ID: "zone-id", Name: "EXAMPLE.com."})
		if err != nil {
			t.Fatalf("TestZone() error = %v", err)
		}
	})

	t.Run("rejects an inactive exact zone", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			writeResponse(t, response, `{"success":true,"result":{"id":"zone-id","name":"example.com","status":"pending"}}`)
		}))
		defer server.Close()

		client := newTestClient(server, "token")
		err := client.TestZone(t.Context(), dnsprovider.Zone{ID: "zone-id", Name: "example.com"})
		if !errors.Is(err, dnsprovider.ErrConflict) {
			t.Fatalf("TestZone(inactive) error = %v, want conflict", err)
		}
	})

	t.Run("reads a supported record and rejects unsafe shapes", func(t *testing.T) {
		responses := []string{
			`{"success":true,"result":[{"id":"record-id","name":"Game.Example.com","type":"A","content":"192.0.2.10","ttl":300,"proxied":false}]}`,
			`{"success":true,"result":[{"id":"record-id","name":"game.example.com","type":"A","content":"192.0.2.10","ttl":300,"proxied":true}]}`,
			`{"success":true,"result":[{"id":"one","name":"game.example.com","type":"A","content":"192.0.2.10","ttl":300,"proxied":false},{"id":"two","name":"game.example.com","type":"A","content":"192.0.2.11","ttl":300,"proxied":false}]}`,
		}
		requestIndex := 0
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			writeResponse(t, response, responses[requestIndex])
			requestIndex++
		}))
		defer server.Close()

		client := newTestClient(server, "token")
		key := dnsprovider.RecordKey{Name: "game.example.com.", Type: dnsprovider.RecordTypeA}
		record, found, err := client.ReadRecord(t.Context(), dnsprovider.Zone{ID: "zone-id"}, key)
		if err != nil || !found {
			t.Fatalf("ReadRecord() = %#v, %v, %v", record, found, err)
		}
		if record.Name != "game.example.com." || record.Value != "192.0.2.10" {
			t.Fatalf("ReadRecord() = %#v", record)
		}

		_, _, err = client.ReadRecord(t.Context(), dnsprovider.Zone{ID: "zone-id"}, key)
		if !errors.Is(err, dnsprovider.ErrUnsupported) {
			t.Fatalf("proxied ReadRecord() error = %v", err)
		}

		_, _, err = client.ReadRecord(t.Context(), dnsprovider.Zone{ID: "zone-id"}, key)
		if !errors.Is(err, dnsprovider.ErrUnsupported) {
			t.Fatalf("multi-value ReadRecord() error = %v", err)
		}
	})

	t.Run("creates and updates DNS-only records", func(t *testing.T) {
		var methods []string
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			methods = append(methods, request.Method)
			var payload struct {
				Type    string `json:"type"`
				Name    string `json:"name"`
				Content string `json:"content"`
				TTL     int64  `json:"ttl"`
				Proxied *bool  `json:"proxied"`
			}
			err := json.NewDecoder(request.Body).Decode(&payload)
			if err != nil {
				t.Fatalf("decoding payload: %v", err)
			}
			if payload.Proxied == nil || *payload.Proxied || payload.Type != "AAAA" || payload.TTL != 300 {
				t.Fatalf("payload = %#v", payload)
			}
			writeResponse(t, response, `{"success":true,"result":{"id":"record-id","name":"game.example.com","type":"AAAA","content":"2001:db8::1","ttl":300,"proxied":false}}`)
		}))
		defer server.Close()

		client := newTestClient(server, "token")
		change := dnsprovider.RecordChange{Name: "game.example.com.", Type: dnsprovider.RecordTypeAAAA, Value: "2001:db8::1", TTL: 300}
		_, err := client.CreateRecord(t.Context(), dnsprovider.Zone{ID: "zone-id"}, change)
		if err != nil {
			t.Fatalf("CreateRecord() error = %v", err)
		}
		_, err = client.UpdateRecord(t.Context(), dnsprovider.Zone{ID: "zone-id"}, dnsprovider.Record{ID: "record-id"}, change)
		if err != nil {
			t.Fatalf("UpdateRecord() error = %v", err)
		}
		if len(methods) != 2 || methods[0] != http.MethodPost || methods[1] != http.MethodPut {
			t.Fatalf("methods = %v", methods)
		}
	})

	t.Run("sanitizes failures", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusUnauthorized)
			writeResponse(t, response, `{"errors":[{"message":"secret provider detail"}]}`)
		}))
		defer server.Close()

		client := newTestClient(server, "token")
		_, err := client.ListZones(t.Context())
		if !errors.Is(err, dnsprovider.ErrUnauthorized) {
			t.Fatalf("ListZones() error = %v", err)
		}
		if strings.Contains(err.Error(), "secret provider detail") {
			t.Fatalf("error leaked provider response: %v", err)
		}
	})
}

func newTestClient(server *httptest.Server, token string) *Client {
	return &Client{token: token, baseURL: server.URL, httpClient: server.Client()}
}

func writeResponse(t *testing.T, response http.ResponseWriter, body string) {
	t.Helper()
	_, err := response.Write([]byte(body))
	if err != nil {
		t.Fatalf("writing test response: %v", err)
	}
}

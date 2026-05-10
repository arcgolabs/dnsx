package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcgolabs/dnsx/dnsserver"
	bboltstore "github.com/arcgolabs/dnsx/dnsserver/store/bbolt"
	"github.com/miekg/dns"
)

func TestAdminHandlerZoneAndRecordFlow(t *testing.T) {
	t.Parallel()

	handler := newAdminHandler(slog.Default(), newTestManager(t))

	zoneResponse := performRequest(t, handler, http.MethodPut, "/zones/example.com", nil)
	if zoneResponse.Code != http.StatusOK {
		t.Fatalf("unexpected zone status: %d", zoneResponse.Code)
	}

	recordPayload := dnsserver.Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.1.7",
	}
	recordBody, err := json.Marshal(recordPayload)
	if err != nil {
		t.Fatalf("marshal record body: %v", err)
	}

	recordResponse := performRequest(t, handler, http.MethodPut, "/records", bytes.NewReader(recordBody))
	if recordResponse.Code != http.StatusOK {
		t.Fatalf("unexpected record upsert status: %d", recordResponse.Code)
	}

	listResponse := performRequest(t, handler, http.MethodGet, "/records?zone=example.com&type=A", nil)
	if listResponse.Code != http.StatusOK {
		t.Fatalf("unexpected record list status: %d", listResponse.Code)
	}

	var listed struct {
		Records []dnsserver.Record `json:"records"`
	}
	decodeResponseJSON(t, listResponse, &listed)
	if len(listed.Records) != 1 || listed.Records[0].Data != "10.0.1.7" {
		t.Fatalf("unexpected listed records: %#v", listed.Records)
	}

	deleteResponse := performRequest(t, handler, http.MethodDelete, "/records", bytes.NewReader(recordBody))
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("unexpected record delete status: %d", deleteResponse.Code)
	}
}

func TestAdminHandlerSeedImport(t *testing.T) {
	t.Parallel()

	handler := newAdminHandler(slog.Default(), newTestManager(t))

	seedBody, err := json.Marshal(dnsserver.SeedData{
		Zones: []dnsserver.Zone{
			{Name: "example.org"},
		},
		Records: []dnsserver.Record{
			{
				Zone: "example.org",
				Name: "example.org",
				TTL:  60,
				Type: dns.TypeSOA,
				Data: "ns1.example.org. hostmaster.example.org. 1 300 60 86400 60",
			},
			{
				Zone: "example.org",
				Name: "example.org",
				TTL:  60,
				Type: dns.TypeNS,
				Data: "ns1.example.org.",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal seed body: %v", err)
	}

	response := performRequest(t, handler, http.MethodPost, "/seed/import", bytes.NewReader(seedBody))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected seed import status: %d", response.Code)
	}

	var result dnsserver.ImportResult
	decodeResponseJSON(t, response, &result)
	if result.Zones != 1 || result.Records != 2 {
		t.Fatalf("unexpected seed import result: %#v", result)
	}
}

func newTestManager(t *testing.T) *dnsserver.Manager {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dnsx.db")
	store, err := bboltstore.Open(path, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close test store: %v", err)
		}
	})

	return dnsserver.NewManager(store)
}

func performRequest(t *testing.T, handler http.Handler, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), method, path, body)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponseJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()

	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
}

//nolint:testpackage // Tests validate internal server wiring without exporting extra API.
package dnsserver

import (
	"context"
	"slices"
	"testing"

	"github.com/miekg/dns"
)

func TestServerTypedLookupHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	server := startLookupTestServer(ctx, t)
	upsertLookupTestRecords(ctx, t, server)

	assertStringLookup(t, "A", []string{"10.0.0.7"}, func() ([]string, error) {
		return server.LookupA(ctx, "www.example.com")
	})
	assertStringLookup(t, "AAAA", []string{"2001:db8::7"}, func() ([]string, error) {
		return server.LookupAAAA(ctx, "v6.example.com")
	})
	assertStringLookup(t, "CNAME", []string{"www.example.com."}, func() ([]string, error) {
		return server.LookupCNAME(ctx, "alias.example.com")
	})
	assertStringLookup(t, "NS", []string{"ns1.example.com."}, func() ([]string, error) {
		return server.LookupNS(ctx, "example.com")
	})
	assertStringLookup(t, "TXT", []string{"hello world"}, func() ([]string, error) {
		return server.LookupTXT(ctx, "txt.example.com")
	})

	assertMXLookup(ctx, t, server)
	assertSRVLookup(ctx, t, server)
}

func startLookupTestServer(ctx context.Context, t *testing.T) *Server {
	t.Helper()

	server := NewServerWithRepository(Config{Listen: "127.0.0.1:0"}, newTestStore(t))
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(ctx); err != nil {
			t.Fatalf("stop server: %v", err)
		}
	})

	return server
}

func upsertLookupTestRecords(ctx context.Context, t *testing.T, server *Server) {
	t.Helper()

	for _, record := range lookupTestRecords() {
		response, _, err := server.UpsertRecord(ctx, record)
		if err != nil {
			t.Fatalf("upsert lookup test record %#v: %v", record, err)
		}
		if response.Rcode != dns.RcodeSuccess {
			t.Fatalf("unexpected update rcode for %#v: %d", record, response.Rcode)
		}
	}
}

func lookupTestRecords() []Record {
	return []Record{
		{Zone: "example.com", Name: "www.example.com", TTL: 60, Type: dns.TypeA, Data: "10.0.0.7"},
		{Zone: "example.com", Name: "v6.example.com", TTL: 60, Type: dns.TypeAAAA, Data: "2001:db8::7"},
		{Zone: "example.com", Name: "alias.example.com", TTL: 60, Type: dns.TypeCNAME, Data: "www.example.com."},
		{Zone: "example.com", Name: "example.com", TTL: 60, Type: dns.TypeNS, Data: "ns1.example.com."},
		{Zone: "example.com", Name: "example.com", TTL: 60, Type: dns.TypeMX, Data: "10 mail.example.com."},
		{Zone: "example.com", Name: "txt.example.com", TTL: 60, Type: dns.TypeTXT, Data: "\"hello world\""},
		{Zone: "example.com", Name: "_api._tcp.example.com", TTL: 60, Type: dns.TypeSRV, Data: "1 5 8443 service.example.com."},
	}
}

func assertStringLookup(t *testing.T, label string, want []string, lookup func() ([]string, error)) {
	t.Helper()

	got, err := lookup()
	if err != nil {
		t.Fatalf("lookup %s: %v", label, err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected %s lookup result: got %#v, want %#v", label, got, want)
	}
}

func assertMXLookup(ctx context.Context, t *testing.T, server *Server) {
	t.Helper()

	got, err := server.LookupMX(ctx, "example.com")
	if err != nil {
		t.Fatalf("lookup MX: %v", err)
	}
	want := []MXRecord{{Host: "mail.example.com.", Preference: 10}}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected MX lookup result: got %#v, want %#v", got, want)
	}
}

func assertSRVLookup(ctx context.Context, t *testing.T, server *Server) {
	t.Helper()

	got, err := server.LookupSRV(ctx, "_api._tcp.example.com")
	if err != nil {
		t.Fatalf("lookup SRV: %v", err)
	}
	want := []SRVRecord{{Target: "service.example.com.", Port: 8443, Priority: 1, Weight: 5}}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected SRV lookup result: got %#v, want %#v", got, want)
	}
}

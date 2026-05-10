//nolint:testpackage // Tests validate internal store behavior without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestMemoryStoreLookup(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()

	if err := store.SaveZone(ctx, Zone{Name: "example.com"}); err != nil {
		t.Fatalf("save zone: %v", err)
	}

	record := Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.1",
	}
	if err := store.SaveRecord(ctx, record); err != nil {
		t.Fatalf("save record: %v", err)
	}

	records, err := store.Lookup(ctx, "example.com", "www.example.com", dns.TypeA, dns.ClassINET)
	if err != nil {
		t.Fatalf("lookup record: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Data != "10.0.0.1" {
		t.Fatalf("unexpected record data: %s", records[0].Data)
	}
}

func newTestStore(t *testing.T) *MemoryStore {
	t.Helper()

	return NewMemoryStore()
}

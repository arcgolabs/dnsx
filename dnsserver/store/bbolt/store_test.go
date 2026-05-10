//nolint:testpackage // Tests validate internal store behavior without exporting extra API.
package bbolt

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"
)

func TestBboltStoreLookup(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	if err := store.SaveZone(ctx, dnsserver.Zone{Name: "example.com"}); err != nil {
		t.Fatalf("save zone: %v", err)
	}

	record := dnsserver.Record{
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

func newTestStore(t *testing.T) *BboltStore {
	t.Helper()

	path := filepath.Join(t.TempDir(), "dnsx.db")
	store, err := OpenBboltStore(path, slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	return store
}

//nolint:testpackage // Tests validate internal manager behavior without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

//nolint:cyclop,gocognit,gocyclo,funlen // End-to-end manager flow is clearer as one scenario.
func TestManagerZoneAndRecordFlow(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	zone, err := manager.UpsertZone(ctx, Zone{Name: "example.com"})
	if err != nil {
		t.Fatalf("upsert zone: %v", err)
	}
	if zone.Name != "example.com." {
		t.Fatalf("unexpected normalized zone: %q", zone.Name)
	}

	record, err := manager.UpsertRecord(ctx, Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.8",
	})
	if err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	if record.Name != "www.example.com." {
		t.Fatalf("unexpected normalized record name: %q", record.Name)
	}

	zones, err := manager.ListZones(ctx)
	if err != nil {
		t.Fatalf("list zones: %v", err)
	}
	if len(zones) != 1 || zones[0].Name != "example.com." {
		t.Fatalf("unexpected zones: %#v", zones)
	}

	records, err := manager.ListRecords(ctx, RecordFilter{Zone: "example.com"})
	if err != nil {
		t.Fatalf("list records by zone: %v", err)
	}
	if len(records) != 1 || records[0].Data != "10.0.0.8" {
		t.Fatalf("unexpected records: %#v", records)
	}

	nameRecords, err := manager.ListRecords(ctx, RecordFilter{
		Zone: "example.com",
		Name: "www.example.com",
		Type: dns.TypeA,
	})
	if err != nil {
		t.Fatalf("list records by name: %v", err)
	}
	if len(nameRecords) != 1 {
		t.Fatalf("expected 1 named record, got %d", len(nameRecords))
	}

	deleteRecordErr := manager.DeleteRecord(ctx, record)
	if deleteRecordErr != nil {
		t.Fatalf("delete record: %v", deleteRecordErr)
	}

	records, err = manager.ListRecords(ctx, RecordFilter{Zone: "example.com"})
	if err != nil {
		t.Fatalf("list records after delete: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records after delete, got %d", len(records))
	}

	deleteZoneErr := manager.DeleteZone(ctx, "example.com")
	if deleteZoneErr != nil {
		t.Fatalf("delete zone: %v", deleteZoneErr)
	}

	zones, err = manager.ListZones(ctx)
	if err != nil {
		t.Fatalf("list zones after delete: %v", err)
	}
	if len(zones) != 0 {
		t.Fatalf("expected no zones after delete, got %d", len(zones))
	}
}

func TestManagerImportSeedData(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	result, err := manager.ImportSeedData(ctx, SeedData{
		Zones: []Zone{
			{Name: "example.com"},
		},
		Records: []Record{
			{
				Zone: "example.com",
				Name: "example.com",
				TTL:  60,
				Type: dns.TypeSOA,
				Data: "ns1.example.com. hostmaster.example.com. 1 300 60 86400 60",
			},
			{
				Zone: "example.com",
				Name: "www.example.com",
				TTL:  60,
				Type: dns.TypeA,
				Data: "10.0.0.9",
			},
		},
	})
	if err != nil {
		t.Fatalf("import seed data: %v", err)
	}
	if result.Zones != 1 || result.Records != 2 {
		t.Fatalf("unexpected import result: %#v", result)
	}

	records, err := manager.ListRecords(ctx, RecordFilter{Zone: "example.com"})
	if err != nil {
		t.Fatalf("list records after import: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records after import, got %d", len(records))
	}
}

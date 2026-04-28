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
				Name: "example.com",
				TTL:  60,
				Type: dns.TypeNS,
				Data: "ns1.example.com.",
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
	if result.Zones != 1 || result.Records != 3 {
		t.Fatalf("unexpected import result: %#v", result)
	}

	records, err := manager.ListRecords(ctx, RecordFilter{Zone: "example.com"})
	if err != nil {
		t.Fatalf("list records after import: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records after import, got %d", len(records))
	}
}

func TestManagerGetZoneAndHasZone(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	if _, err := manager.UpsertZone(ctx, Zone{Name: "example.net"}); err != nil {
		t.Fatalf("upsert zone: %v", err)
	}

	zone, err := manager.GetZone(ctx, "example.net")
	if err != nil {
		t.Fatalf("get zone: %v", err)
	}
	if zone.IsAbsent() {
		t.Fatal("expected zone to exist")
	}

	hasZone, err := manager.HasZone(ctx, "example.net")
	if err != nil {
		t.Fatalf("has zone: %v", err)
	}
	if !hasZone {
		t.Fatal("expected zone existence check to be true")
	}
}

//nolint:cyclop,gocognit,gocyclo // RRSet lifecycle assertions are intentionally exercised together.
func TestManagerRRSetAndDeleteNameFlow(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	if _, err := manager.UpsertRecord(ctx, Record{
		Zone: "example.org",
		Name: "api.example.org",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.1",
	}); err != nil {
		t.Fatalf("upsert seed record: %v", err)
	}

	savedRecords, err := manager.UpsertRRSet(ctx, "example.org", "api.example.org", dns.TypeA, []Record{
		{TTL: 60, Data: "10.0.0.2"},
		{TTL: 60, Data: "10.0.0.3"},
	})
	if err != nil {
		t.Fatalf("upsert rrset: %v", err)
	}
	if len(savedRecords) != 2 {
		t.Fatalf("expected 2 saved rrset records, got %d", len(savedRecords))
	}

	records, err := manager.GetRecords(ctx, RecordFilter{
		Zone: "example.org",
		Name: "api.example.org",
		Type: dns.TypeA,
	})
	if err != nil {
		t.Fatalf("get rrset records: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rrset records, got %d", len(records))
	}
	if records[0].Data != "10.0.0.2" || records[1].Data != "10.0.0.3" {
		t.Fatalf("unexpected rrset data: %#v", records)
	}

	deletedRRSet, err := manager.DeleteRRSet(ctx, "example.org", "api.example.org", dns.TypeA)
	if err != nil {
		t.Fatalf("delete rrset: %v", err)
	}
	if deletedRRSet != 2 {
		t.Fatalf("expected 2 deleted rrset records, got %d", deletedRRSet)
	}

	_, upsertNameAErr := manager.UpsertRecord(ctx, Record{
		Zone: "example.org",
		Name: "api.example.org",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.4",
	})
	if upsertNameAErr != nil {
		t.Fatalf("upsert name record: %v", upsertNameAErr)
	}
	_, upsertNameTXTErr := manager.UpsertRecord(ctx, Record{
		Zone: "example.org",
		Name: "api.example.org",
		TTL:  60,
		Type: dns.TypeTXT,
		Data: "\"v1\"",
	})
	if upsertNameTXTErr != nil {
		t.Fatalf("upsert second name record: %v", upsertNameTXTErr)
	}

	deletedName, err := manager.DeleteName(ctx, "example.org", "api.example.org")
	if err != nil {
		t.Fatalf("delete name: %v", err)
	}
	if deletedName != 2 {
		t.Fatalf("expected 2 deleted name records, got %d", deletedName)
	}
}

func TestManagerApplyChanges(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	result, err := manager.ApplyChanges(ctx, []Change{
		{Kind: ChangeUpsertZone, ZoneName: "example.io"},
		{
			Kind:     ChangeUpsertRRSet,
			ZoneName: "example.io",
			Name:     "www.example.io",
			Type:     dns.TypeA,
			Records: []Record{
				{TTL: 120, Data: "10.0.1.1"},
				{TTL: 120, Data: "10.0.1.2"},
			},
		},
		{Kind: ChangeDeleteName, ZoneName: "example.io", Name: "www.example.io"},
	})
	if err != nil {
		t.Fatalf("apply changes: %v", err)
	}

	if result.Applied != 3 || result.ZonesUpserted != 1 || result.RecordsUpserted != 2 || result.RecordsDeleted != 2 {
		t.Fatalf("unexpected change result: %#v", result)
	}

	records, err := manager.GetRecords(ctx, RecordFilter{Zone: "example.io"})
	if err != nil {
		t.Fatalf("list records after changes: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no records after changes, got %d", len(records))
	}
}

//nolint:testpackage // Tests validate internal manager behavior without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestManagerPreviewChangesDoesNotMutateStore(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	changes := []Change{
		{Kind: ChangeUpsertZone, ZoneName: "preview.example"},
		{
			Kind:     ChangeUpsertRRSet,
			ZoneName: "preview.example",
			Name:     "www.preview.example",
			Type:     dns.TypeA,
			Records: []Record{
				{TTL: 120, Data: "10.0.2.1"},
				{TTL: 120, Data: "10.0.2.2"},
			},
		},
	}

	if err := manager.ValidateChanges(ctx, changes); err != nil {
		t.Fatalf("validate changes: %v", err)
	}

	preview, err := manager.PreviewChanges(ctx, changes)
	if err != nil {
		t.Fatalf("preview changes: %v", err)
	}
	if preview.Changes != 2 {
		t.Fatalf("expected 2 previewed changes, got %d", preview.Changes)
	}
	if len(preview.Zones) != 1 {
		t.Fatalf("expected 1 preview zone, got %d", len(preview.Zones))
	}
	if got := len(preview.Zones[0].Records); got != 2 {
		t.Fatalf("expected 2 preview records, got %d", got)
	}

	records, err := manager.GetRecords(ctx, RecordFilter{Zone: "preview.example"})
	if err != nil {
		t.Fatalf("list records after preview: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected preview to avoid mutating records, got %d", len(records))
	}
}

func TestManagerPreviewChangesReportsDeletedZones(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	if _, err := manager.UpsertRecord(ctx, Record{
		Zone: "delete-preview.example",
		Name: "www.delete-preview.example",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.3.1",
	}); err != nil {
		t.Fatalf("upsert record: %v", err)
	}

	preview, err := manager.PreviewChanges(ctx, []Change{
		{Kind: ChangeDeleteZone, ZoneName: "delete-preview.example"},
	})
	if err != nil {
		t.Fatalf("preview delete zone: %v", err)
	}
	if len(preview.DeletedZones) != 1 || preview.DeletedZones[0] != "delete-preview.example." {
		t.Fatalf("unexpected deleted zones preview: %#v", preview.DeletedZones)
	}

	hasZone, err := manager.HasZone(ctx, "delete-preview.example")
	if err != nil {
		t.Fatalf("has zone after preview: %v", err)
	}
	if !hasZone {
		t.Fatal("expected preview to avoid deleting zone")
	}
}

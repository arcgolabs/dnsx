//nolint:testpackage // Tests validate internal manager write constraints without exporting extra API.
package dnsserver

import (
	"context"
	"errors"
	"testing"

	"github.com/miekg/dns"
)

func TestManagerRejectsCNAMEConflicts(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	if _, err := manager.UpsertRecord(ctx, Record{
		Zone: "example.dev",
		Name: "alias.example.dev",
		TTL:  60,
		Type: dns.TypeCNAME,
		Data: "target.example.dev.",
	}); err != nil {
		t.Fatalf("upsert cname: %v", err)
	}

	_, err := manager.UpsertRecord(ctx, Record{
		Zone: "example.dev",
		Name: "alias.example.dev",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.30",
	})
	if !errors.Is(err, ErrCNAMEConflict) {
		t.Fatalf("expected cname conflict, got %v", err)
	}
	if !HasErrorCode(err, CodeZoneCNAMEConflict) {
		t.Fatalf("expected cname conflict code, got %v", err)
	}
}

func TestManagerRejectsSOAOutsideApex(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	_, err := manager.UpsertRecord(ctx, Record{
		Zone: "example.app",
		Name: "ns1.example.app",
		TTL:  60,
		Type: dns.TypeSOA,
		Data: "ns1.example.app. hostmaster.example.app. 1 300 60 86400 60",
	})
	if !errors.Is(err, ErrSOAMustBeAtZoneApex) {
		t.Fatalf("expected soa apex error, got %v", err)
	}
	if !HasErrorCode(err, CodeZoneSOANotAtApex) {
		t.Fatalf("expected soa apex code, got %v", err)
	}
}

func TestManagerApplyChangesRejectsMissingApexNS(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	_, err := manager.ApplyChanges(ctx, []Change{
		{Kind: ChangeUpsertZone, ZoneName: "example.ops"},
		{
			Kind:     ChangeUpsertRRSet,
			ZoneName: "example.ops",
			Name:     "example.ops",
			Type:     dns.TypeSOA,
			Records: []Record{
				{TTL: 300, Data: "ns1.example.ops. hostmaster.example.ops. 1 300 60 86400 60"},
			},
		},
	})
	if !errors.Is(err, ErrApexNSRequired) {
		t.Fatalf("expected apex ns error, got %v", err)
	}
	if !HasErrorCode(err, CodeZoneApexNSRequired) {
		t.Fatalf("expected apex ns code, got %v", err)
	}
}

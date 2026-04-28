//nolint:testpackage // Tests validate internal manager read models without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestManagerGetZoneSnapshotAndListRRSets(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	manager := NewManager(store)
	ctx := context.Background()

	for _, record := range []Record{
		{
			Zone: "example.edu",
			Name: "example.edu",
			TTL:  300,
			Type: dns.TypeSOA,
			Data: "ns1.example.edu. hostmaster.example.edu. 1 300 60 86400 60",
		},
		{
			Zone: "example.edu",
			Name: "example.edu",
			TTL:  300,
			Type: dns.TypeNS,
			Data: "ns1.example.edu.",
		},
		{
			Zone: "example.edu",
			Name: "www.example.edu",
			TTL:  60,
			Type: dns.TypeA,
			Data: "10.0.0.20",
		},
		{
			Zone: "example.edu",
			Name: "www.example.edu",
			TTL:  60,
			Type: dns.TypeA,
			Data: "10.0.0.21",
		},
	} {
		if _, err := manager.UpsertRecord(ctx, record); err != nil {
			t.Fatalf("upsert snapshot record %+v: %v", record, err)
		}
	}

	snapshot, err := manager.GetZoneSnapshot(ctx, "example.edu")
	if err != nil {
		t.Fatalf("get zone snapshot: %v", err)
	}
	if snapshot.IsAbsent() {
		t.Fatal("expected snapshot to exist")
	}

	snapshotValue := snapshot.MustGet()
	if validateErr := snapshotValue.Validate(); validateErr != nil {
		t.Fatalf("validate zone snapshot: %v", validateErr)
	}
	if got := len(snapshotValue.RRSets); got != 3 {
		t.Fatalf("expected 3 rrsets in snapshot, got %d", got)
	}

	rrsets, err := manager.ListRRSets(ctx, "example.edu", "www.example.edu")
	if err != nil {
		t.Fatalf("list rrsets: %v", err)
	}
	if len(rrsets) != 1 {
		t.Fatalf("expected 1 rrset, got %d", len(rrsets))
	}
	if len(rrsets[0].Records) != 2 {
		t.Fatalf("expected 2 records in rrset, got %d", len(rrsets[0].Records))
	}
}

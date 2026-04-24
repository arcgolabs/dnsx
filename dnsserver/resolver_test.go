//nolint:testpackage // Tests validate internal resolver behavior without exporting extra API.
package dnsserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestResolverUsesRevisionToAvoidStaleCache(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "example.com",
		TTL:  60,
		Type: dns.TypeSOA,
		Data: "ns1.example.com. hostmaster.example.com. 1 300 60 86400 60",
	})
	oldRecord := Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.1",
	}
	mustSaveRecord(ctx, t, store, oldRecord)

	resolver := NewResolver(store, WithHotCache(8, time.Minute))

	first, err := resolver.Resolve(ctx, dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if got := first.Answer[0].String(); got == "" || !strings.Contains(got, "10.0.0.1") {
		t.Fatalf("unexpected first answer: %s", got)
	}

	deleteErr := store.DeleteRecord(ctx, oldRecord)
	if deleteErr != nil {
		t.Fatalf("delete old record: %v", deleteErr)
	}
	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.2",
	})

	second, err := resolver.Resolve(ctx, dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if got := second.Answer[0].String(); !strings.Contains(got, "10.0.0.2") {
		t.Fatalf("expected refreshed answer, got: %s", got)
	}
}

func TestResolverReturnsCNAMEChain(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()

	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "example.com",
		TTL:  60,
		Type: dns.TypeSOA,
		Data: "ns1.example.com. hostmaster.example.com. 1 300 60 86400 60",
	})
	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "alias.example.com",
		TTL:  60,
		Type: dns.TypeCNAME,
		Data: "www.example.com.",
	})
	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.3",
	})

	resolver := NewResolver(store)
	response, err := resolver.Resolve(ctx, dns.Question{Name: "alias.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET})
	if err != nil {
		t.Fatalf("resolve cname: %v", err)
	}
	if len(response.Answer) != 2 {
		t.Fatalf("expected cname + a answer, got %d", len(response.Answer))
	}
}

func mustSaveRecord(ctx context.Context, t *testing.T, store *BboltStore, record Record) {
	t.Helper()
	if err := store.SaveRecord(ctx, record); err != nil {
		t.Fatalf("save record %+v: %v", record, err)
	}
}

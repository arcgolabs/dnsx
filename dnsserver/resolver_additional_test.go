//nolint:testpackage // Tests validate internal resolver behavior without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestResolverAddsAdditionalRecordsForInZoneTargets(t *testing.T) {
	t.Parallel()

	assertAdditionalRecord(t,
		Record{Zone: "example.com", Name: "example.com", TTL: 60, Type: dns.TypeNS, Data: "ns1.example.com."},
		Record{Zone: "example.com", Name: "ns1.example.com", TTL: 60, Type: dns.TypeA, Data: "10.0.0.53"},
		dns.TypeNS,
		dns.TypeA,
	)
}

func TestResolverAddsAdditionalRecordsForMXAnswer(t *testing.T) {
	t.Parallel()

	assertAdditionalRecord(t,
		Record{Zone: "example.com", Name: "example.com", TTL: 60, Type: dns.TypeMX, Data: "10 mail.example.com."},
		Record{Zone: "example.com", Name: "mail.example.com", TTL: 60, Type: dns.TypeAAAA, Data: "2001:db8::25"},
		dns.TypeMX,
		dns.TypeAAAA,
	)
}

func TestResolverSkipsOutOfZoneAdditionalTargets(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	ctx := context.Background()
	seedAdditionalRecords(ctx, t, store, Record{
		Zone: "example.com",
		Name: "example.com",
		TTL:  60,
		Type: dns.TypeNS,
		Data: "ns1.example.net.",
	})

	resolver := NewResolver(store)
	response, err := resolver.Resolve(ctx, dns.Question{Name: "example.com.", Qtype: dns.TypeNS, Qclass: dns.ClassINET})
	if err != nil {
		t.Fatalf("resolve ns: %v", err)
	}
	if len(response.Extra) != 0 {
		t.Fatalf("expected no additional records, got %#v", response.Extra)
	}
}

func seedAdditionalRecords(ctx context.Context, t *testing.T, store *BboltStore, records ...Record) {
	t.Helper()
	mustSaveRecord(ctx, t, store, Record{
		Zone: "example.com",
		Name: "example.com",
		TTL:  60,
		Type: dns.TypeSOA,
		Data: "ns1.example.com. hostmaster.example.com. 1 300 60 86400 60",
	})
	for _, record := range records {
		mustSaveRecord(ctx, t, store, record)
	}
}

func assertAdditionalRecord(t *testing.T, answer, address Record, queryType, extraType uint16) {
	t.Helper()

	store := newTestStore(t)
	ctx := context.Background()
	seedAdditionalRecords(ctx, t, store, answer, address)

	resolver := NewResolver(store)
	response, err := resolver.Resolve(ctx, dns.Question{Name: "example.com.", Qtype: queryType, Qclass: dns.ClassINET})
	if err != nil {
		t.Fatalf("resolve type %d: %v", queryType, err)
	}
	if len(response.Answer) != 1 || response.Answer[0].Header().Rrtype != queryType {
		t.Fatalf("expected answer type %d, got %#v", queryType, response.Answer)
	}
	if len(response.Extra) != 1 || response.Extra[0].Header().Rrtype != extraType {
		t.Fatalf("expected extra type %d, got %#v", extraType, response.Extra)
	}
}

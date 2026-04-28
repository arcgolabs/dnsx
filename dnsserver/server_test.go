//nolint:testpackage // Tests validate internal server wiring without exporting extra API.
package dnsserver

import (
	"context"
	"testing"

	"github.com/miekg/dns"
)

func TestServerStartStop(t *testing.T) {
	t.Parallel()

	server := NewServer(
		Config{Listen: "127.0.0.1:0"},
		dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
			reply := new(dns.Msg)
			reply.SetReply(request)
			if err := writer.WriteMsg(reply); err != nil {
				t.Fatalf("write reply: %v", err)
			}
		}),
	)

	if err := server.Start(context.Background()); err != nil {
		t.Fatalf("start server: %v", err)
	}
	if server.UDPAddr() == "" {
		t.Fatal("expected udp address after start")
	}
	if server.TCPAddr() == "" {
		t.Fatal("expected tcp address after start")
	}
	if err := server.Stop(context.Background()); err != nil {
		t.Fatalf("stop server: %v", err)
	}
}

//nolint:cyclop,gocognit,gocyclo // End-to-end flow assertions are intentionally kept together.
func TestServerInternalClientQueryAndUpdateFlow(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	server := NewServerWithRepository(
		Config{Listen: "127.0.0.1:0"},
		store,
	)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Fatalf("stop server: %v", err)
		}
	})

	record := Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.7",
	}
	updateResponse, _, err := server.UpsertRecord(ctx, record)
	if err != nil {
		t.Fatalf("upsert record via dns update: %v", err)
	}
	if updateResponse.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected update rcode: %d", updateResponse.Rcode)
	}

	records, err := store.Lookup(ctx, "example.com", "www.example.com", dns.TypeA, dns.ClassINET)
	if err != nil {
		t.Fatalf("lookup record in store after update: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 stored record after update, got %d", len(records))
	}

	answers, err := server.LookupA(ctx, "www.example.com")
	if err != nil {
		t.Fatalf("lookup a via internal client: %v", err)
	}
	if len(answers) != 1 || answers[0] != "10.0.0.7" {
		t.Fatalf("unexpected answers: %#v", answers)
	}

	deleteResponse, _, err := server.DeleteRecord(ctx, record)
	if err != nil {
		t.Fatalf("delete record via dns update: %v", err)
	}
	if deleteResponse.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected delete rcode: %d", deleteResponse.Rcode)
	}

	records, err = store.Lookup(ctx, "example.com", "www.example.com", dns.TypeA, dns.ClassINET)
	if err != nil {
		t.Fatalf("lookup record in store after delete: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected 0 stored record after delete, got %d", len(records))
	}

	response, _, err := server.Query(ctx, "www.example.com", dns.TypeA)
	if err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if len(response.Answer) != 0 {
		t.Fatalf("expected no answer after delete, got %d", len(response.Answer))
	}
}

//nolint:cyclop,gocognit,gocyclo // End-to-end RRSet flow is clearer as one scenario.
func TestServerInternalClientRRSetFlow(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	server := NewServerWithRepository(
		Config{Listen: "127.0.0.1:0"},
		store,
	)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			t.Fatalf("stop server: %v", err)
		}
	})

	response, _, err := server.UpsertRRSet(ctx, "example.com", "api.example.com", dns.TypeA, []Record{
		{TTL: 60, Data: "10.0.0.10"},
		{TTL: 60, Data: "10.0.0.11"},
	})
	if err != nil {
		t.Fatalf("upsert rrset via dns update: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected rrset update rcode: %d", response.Rcode)
	}

	records, err := store.Lookup(ctx, "example.com", "api.example.com", dns.TypeA, dns.ClassINET)
	if err != nil {
		t.Fatalf("lookup rrset in store: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 rrset records, got %d", len(records))
	}

	deleteResponse, _, err := server.DeleteRRSet(ctx, "example.com", "api.example.com", dns.TypeA)
	if err != nil {
		t.Fatalf("delete rrset via dns update: %v", err)
	}
	if deleteResponse.Rcode != dns.RcodeSuccess {
		t.Fatalf("unexpected rrset delete rcode: %d", deleteResponse.Rcode)
	}

	records, err = store.Lookup(ctx, "example.com", "api.example.com", dns.TypeA, dns.ClassINET)
	if err != nil {
		t.Fatalf("lookup rrset after delete: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no rrset records after delete, got %d", len(records))
	}
}

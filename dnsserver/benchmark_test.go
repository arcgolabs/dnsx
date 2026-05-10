//nolint:testpackage // Benchmarks exercise package internals without widening the public API.
package dnsserver

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func BenchmarkResolverQueryAHit(b *testing.B) {
	benchmarkResolverQuery(b, dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}, 1)
}

func BenchmarkResolverQueryAHitParallel(b *testing.B) {
	ctx := context.Background()
	resolver := newBenchmarkResolver(b)
	question := dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			assertResolverAnswerCount(ctx, b, resolver, question, 1)
		}
	})
}

func BenchmarkResolverQueryCNAMEChain(b *testing.B) {
	benchmarkResolverQuery(b, dns.Question{Name: "alias.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}, 2)
}

func BenchmarkResolverQueryNXDomain(b *testing.B) {
	ctx := context.Background()
	resolver := newBenchmarkResolver(b)
	question := dns.Question{Name: "missing.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		resolution, err := resolver.Resolve(ctx, question)
		if err != nil {
			b.Fatalf("resolve nxdomain: %v", err)
		}
		if resolution.RCode != dns.RcodeNameError {
			b.Fatalf("expected NXDOMAIN, got %d", resolution.RCode)
		}
	}
}

func BenchmarkServerQueryInternalClientA(b *testing.B) {
	benchmarkServerQuery(b, false)
}

func BenchmarkServerQueryInternalClientAParallel(b *testing.B) {
	benchmarkServerQuery(b, true)
}

func BenchmarkServerUpdateUpsertDelete(b *testing.B) {
	benchmarkServerUpdate(b, false)
}

func BenchmarkServerUpdateUpsertDeleteParallel(b *testing.B) {
	benchmarkServerUpdate(b, true)
}

func benchmarkResolverQuery(b *testing.B, question dns.Question, expectedAnswers int) {
	b.Helper()

	ctx := context.Background()
	resolver := newBenchmarkResolver(b)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		assertResolverAnswerCount(ctx, b, resolver, question, expectedAnswers)
	}
}

func benchmarkServerQuery(b *testing.B, parallel bool) {
	b.Helper()

	ctx := context.Background()
	server := newBenchmarkServer(b)

	b.ReportAllocs()
	b.ResetTimer()
	if !parallel {
		for range b.N {
			assertServerAnswerCount(ctx, b, server, "www.example.com", 1)
		}
		return
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			assertServerAnswerCount(ctx, b, server, "www.example.com", 1)
		}
	})
}

func benchmarkServerUpdate(b *testing.B, parallel bool) {
	b.Helper()

	ctx := context.Background()
	server := newBenchmarkServer(b)
	var counter atomic.Uint64

	b.ReportAllocs()
	b.ResetTimer()
	if !parallel {
		for range b.N {
			benchmarkServerUpdateCycle(ctx, b, server, benchmarkRecord(counter.Add(1)))
		}
		return
	}

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			benchmarkServerUpdateCycle(ctx, b, server, benchmarkRecord(counter.Add(1)))
		}
	})
}

func benchmarkServerUpdateCycle(ctx context.Context, b *testing.B, server *Server, record Record) {
	b.Helper()

	response, _, err := server.UpsertRecord(ctx, record)
	if err != nil {
		b.Fatalf("upsert record: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		b.Fatalf("unexpected upsert rcode: %d", response.Rcode)
	}

	response, _, err = server.DeleteRecord(ctx, record)
	if err != nil {
		b.Fatalf("delete record: %v", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		b.Fatalf("unexpected delete rcode: %d", response.Rcode)
	}
}

func assertResolverAnswerCount(ctx context.Context, b *testing.B, resolver *Resolver, question dns.Question, expected int) {
	b.Helper()

	resolution, err := resolver.Resolve(ctx, question)
	if err != nil {
		b.Fatalf("resolve %s: %v", question.Name, err)
	}
	if len(resolution.Answer) != expected {
		b.Fatalf("expected %d answers, got %d", expected, len(resolution.Answer))
	}
}

func assertServerAnswerCount(ctx context.Context, b *testing.B, server *Server, name string, expected int) {
	b.Helper()

	response, _, err := server.Query(ctx, name, dns.TypeA)
	if err != nil {
		b.Fatalf("server query %s: %v", name, err)
	}
	if len(response.Answer) != expected {
		b.Fatalf("expected %d answers, got %d", expected, len(response.Answer))
	}
}

func newBenchmarkResolver(b *testing.B) *Resolver {
	b.Helper()

	ctx := context.Background()
	store := newBenchmarkStore(b)
	seedBenchmarkStore(ctx, b, store)

	return NewResolver(
		store,
		WithResolverLogger(benchmarkLogger()),
		WithHotCache(4096, time.Minute),
		WithNegativeCacheTTL(time.Minute),
	)
}

func newBenchmarkServer(b *testing.B) *Server {
	b.Helper()

	ctx := context.Background()
	store := newBenchmarkStore(b)
	seedBenchmarkStore(ctx, b, store)

	resolver := NewResolver(
		store,
		WithResolverLogger(benchmarkLogger()),
		WithHotCache(4096, time.Minute),
		WithNegativeCacheTTL(time.Minute),
	)

	server := NewServerWithResolver(
		Config{Listen: "127.0.0.1:0"},
		resolver,
		WithLogger(benchmarkLogger()),
	)
	if err := server.Start(ctx); err != nil {
		b.Fatalf("start benchmark server: %v", err)
	}
	b.Cleanup(func() {
		if err := server.Stop(context.Background()); err != nil {
			b.Fatalf("stop benchmark server: %v", err)
		}
	})

	return server
}

func newBenchmarkStore(b *testing.B) *MemoryStore {
	b.Helper()

	return NewMemoryStore()
}

func seedBenchmarkStore(ctx context.Context, b *testing.B, store Repository) {
	b.Helper()

	for _, record := range []Record{
		{
			Zone: "example.com",
			Name: "example.com",
			TTL:  300,
			Type: dns.TypeSOA,
			Data: "ns1.example.com. hostmaster.example.com. 1 300 60 86400 60",
		},
		{
			Zone: "example.com",
			Name: "www.example.com",
			TTL:  60,
			Type: dns.TypeA,
			Data: "10.0.0.10",
		},
		{
			Zone: "example.com",
			Name: "alias.example.com",
			TTL:  60,
			Type: dns.TypeCNAME,
			Data: "www.example.com.",
		},
	} {
		if err := store.SaveRecord(ctx, record); err != nil {
			b.Fatalf("seed benchmark record %+v: %v", record, err)
		}
	}
}

func benchmarkRecord(index uint64) Record {
	name := fmt.Sprintf("bench-%d.example.com", index)
	return Record{
		Zone: "example.com",
		Name: name,
		TTL:  30,
		Type: dns.TypeA,
		Data: "10.0.1.10",
	}
}

func benchmarkLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

package dnsserver

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func BenchmarkResolverQuery(b *testing.B) {
	ctx := context.Background()
	store := newBenchmarkStore(b)
	seedBenchmarkStore(b, ctx, store)

	resolver := NewResolver(
		store,
		WithResolverLogger(benchmarkLogger()),
		WithHotCache(4096, time.Minute),
		WithNegativeCacheTTL(time.Minute),
	)

	b.Run("AHit", func(b *testing.B) {
		question := dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			resolution, err := resolver.Resolve(ctx, question)
			if err != nil {
				b.Fatalf("resolve a hit: %v", err)
			}
			if len(resolution.Answer) != 1 {
				b.Fatalf("expected 1 answer, got %d", len(resolution.Answer))
			}
		}
	})

	b.Run("AHitParallel", func(b *testing.B) {
		question := dns.Question{Name: "www.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				resolution, err := resolver.Resolve(ctx, question)
				if err != nil {
					b.Fatalf("resolve a hit parallel: %v", err)
				}
				if len(resolution.Answer) != 1 {
					b.Fatalf("expected 1 answer, got %d", len(resolution.Answer))
				}
			}
		})
	})

	b.Run("CNAMEChain", func(b *testing.B) {
		question := dns.Question{Name: "alias.example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			resolution, err := resolver.Resolve(ctx, question)
			if err != nil {
				b.Fatalf("resolve cname chain: %v", err)
			}
			if len(resolution.Answer) != 2 {
				b.Fatalf("expected 2 answers, got %d", len(resolution.Answer))
			}
		}
	})

	b.Run("NXDomain", func(b *testing.B) {
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
	})
}

func BenchmarkServerQuery(b *testing.B) {
	ctx := context.Background()
	server := newBenchmarkServer(b)

	b.Run("InternalClientA", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			response, _, err := server.Query(ctx, "www.example.com", dns.TypeA)
			if err != nil {
				b.Fatalf("server query a: %v", err)
			}
			if len(response.Answer) != 1 {
				b.Fatalf("expected 1 answer, got %d", len(response.Answer))
			}
		}
	})

	b.Run("InternalClientAParallel", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				response, _, err := server.Query(ctx, "www.example.com", dns.TypeA)
				if err != nil {
					b.Fatalf("server query a parallel: %v", err)
				}
				if len(response.Answer) != 1 {
					b.Fatalf("expected 1 answer, got %d", len(response.Answer))
				}
			}
		})
	})
}

func BenchmarkServerUpdate(b *testing.B) {
	ctx := context.Background()
	server := newBenchmarkServer(b)

	b.Run("UpsertDelete", func(b *testing.B) {
		var counter atomic.Uint64
		b.ReportAllocs()
		b.ResetTimer()

		for range b.N {
			record := benchmarkRecord(counter.Add(1))
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
	})

	b.Run("UpsertDeleteParallel", func(b *testing.B) {
		var counter atomic.Uint64
		b.ReportAllocs()
		b.ResetTimer()

		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				record := benchmarkRecord(counter.Add(1))
				response, _, err := server.UpsertRecord(ctx, record)
				if err != nil {
					b.Fatalf("parallel upsert record: %v", err)
				}
				if response.Rcode != dns.RcodeSuccess {
					b.Fatalf("unexpected parallel upsert rcode: %d", response.Rcode)
				}

				response, _, err = server.DeleteRecord(ctx, record)
				if err != nil {
					b.Fatalf("parallel delete record: %v", err)
				}
				if response.Rcode != dns.RcodeSuccess {
					b.Fatalf("unexpected parallel delete rcode: %d", response.Rcode)
				}
			}
		})
	})
}

func newBenchmarkServer(b *testing.B) *Server {
	b.Helper()

	ctx := context.Background()
	store := newBenchmarkStore(b)
	seedBenchmarkStore(b, ctx, store)

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
		_ = server.Stop(context.Background())
		_ = store.Close()
	})

	return server
}

func newBenchmarkStore(b *testing.B) *BboltStore {
	b.Helper()

	path := filepath.Join(b.TempDir(), "dnsx-bench.db")
	store, err := OpenBboltStore(path, benchmarkLogger())
	if err != nil {
		b.Fatalf("open benchmark store: %v", err)
	}
	b.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

func seedBenchmarkStore(b *testing.B, ctx context.Context, store *BboltStore) {
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
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

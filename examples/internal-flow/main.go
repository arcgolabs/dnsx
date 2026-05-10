// Package main demonstrates dnsserver internal client helpers for query and update flows.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arcgolabs/dnsx/dnsserver"
	bboltstore "github.com/arcgolabs/dnsx/dnsserver/store/bbolt"
	"github.com/miekg/dns"
)

func main() {
	ctx := context.Background()
	logger := slog.Default()

	workdir, err := os.MkdirTemp("", "dnsx-example-internal-*")
	must(err)
	defer func() { must(os.RemoveAll(workdir)) }()

	store, err := bboltstore.Open(filepath.Join(workdir, "dnsx.db"), logger)
	must(err)
	defer func() { must(store.Close()) }()

	server := dnsserver.NewServerWithRepository(
		dnsserver.Config{Listen: "127.0.0.1:0"},
		store,
		dnsserver.WithLogger(logger),
	)
	must(server.Start(ctx))
	defer func() { must(server.Stop(ctx)) }()

	record := dnsserver.Record{
		Zone: "example.com",
		Name: "api.example.com",
		TTL:  120,
		Type: dns.TypeA,
		Data: "10.0.0.21",
	}

	_, _, err = server.UpsertRecord(ctx, record)
	must(err)

	response, _, err := server.Query(ctx, "api.example.com", dns.TypeA)
	must(err)

	mustPrint("answers after upsert: %d\n", len(response.Answer))

	_, _, err = server.DeleteRecord(ctx, record)
	must(err)

	response, _, err = server.Query(ctx, "api.example.com", dns.TypeA)
	must(err)

	mustPrint("answers after delete: %d\n", len(response.Answer))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustPrint(format string, values ...any) {
	_, err := fmt.Printf(format, values...)
	must(err)
}

// Package main demonstrates embedding dnsserver as an authoritative DNS component.
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

	workdir, err := os.MkdirTemp("", "dnsx-example-embedded-*")
	must(err)
	defer func() { must(os.RemoveAll(workdir)) }()

	store, err := bboltstore.Open(filepath.Join(workdir, "dnsx.db"), logger)
	must(err)
	defer func() { must(store.Close()) }()

	must(store.SaveRecord(ctx, dnsserver.Record{
		Zone: "example.com",
		Name: "www.example.com",
		TTL:  60,
		Type: dns.TypeA,
		Data: "10.0.0.10",
	}))

	resolver := dnsserver.NewResolver(
		store,
		dnsserver.WithResolverLogger(logger),
	)

	server := dnsserver.NewServerWithResolver(
		dnsserver.Config{Listen: "127.0.0.1:0"},
		resolver,
		dnsserver.WithLogger(logger),
	)
	must(server.Start(ctx))
	defer func() { must(server.Stop(ctx)) }()

	answers, err := server.LookupA(ctx, "www.example.com")
	must(err)

	mustPrint("server listening on %s\n", server.UDPAddr())
	mustPrint("www.example.com -> %v\n", answers)
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

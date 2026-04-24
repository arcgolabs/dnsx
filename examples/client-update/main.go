package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/arcgolabs/dnsx/dnsclient"
	"github.com/arcgolabs/dnsx/dnsserver"
	"github.com/miekg/dns"
)

func main() {
	ctx := context.Background()
	logger := slog.Default()

	workdir, err := os.MkdirTemp("", "dnsx-example-client-*")
	must(err)
	defer os.RemoveAll(workdir)

	store, err := dnsserver.OpenBboltStore(filepath.Join(workdir, "dnsx.db"), logger)
	must(err)
	defer store.Close()

	server := dnsserver.NewServerWithRepository(
		dnsserver.Config{Listen: "127.0.0.1:0"},
		store,
		dnsserver.WithLogger(logger),
	)
	must(server.Start(ctx))
	defer server.Stop(ctx)

	client := dnsclient.NewClient(server.UDPAddr())
	record, err := dns.NewRR("ops.example.com. 60 IN A 10.0.0.31")
	must(err)

	updateResponse, _, err := client.UpdateAdd(ctx, "example.com", record)
	must(err)
	fmt.Printf("update rcode: %s\n", dns.RcodeToString[updateResponse.Rcode])

	answer, err := client.Lookup(ctx, "ops.example.com", dns.TypeA)
	must(err)
	fmt.Printf("answer count: %d\n", len(answer))

	deleteResponse, _, err := client.UpdateRemove(ctx, "example.com", record)
	must(err)
	fmt.Printf("delete rcode: %s\n", dns.RcodeToString[deleteResponse.Rcode])
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

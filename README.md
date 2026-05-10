# dnsx

`dnsx` is a Go workspace for building authoritative DNS components on top of [`miekg/dns`](https://github.com/miekg/dns).

The repository is split into versioned modules:

- `github.com/arcgolabs/dnsx`:
  repository root module for examples and workspace entrypoints
- `github.com/arcgolabs/dnsx/dnsclient`:
  DNS client helpers for query and RFC2136 update flows
- `github.com/arcgolabs/dnsx/dnsserver`:
  embeddable authoritative DNS server with memory storage, caching, manager APIs, and internal client flows
- `github.com/arcgolabs/dnsx/dnsserver/store/bbolt`:
  optional bbolt-backed persistence for `dnsserver`
- `github.com/arcgolabs/dnsx/cmd/server`:
  standalone DNS server process wired with `dix`, `configx`, `logx`, and bbolt persistence

## Install

Library modules:

```bash
go get github.com/arcgolabs/dnsx/dnsclient@v0.1.4
go get github.com/arcgolabs/dnsx/dnsserver@v0.1.4
go get github.com/arcgolabs/dnsx/dnsserver/store/bbolt@v0.1.4
```

Standalone server:

```bash
go install github.com/arcgolabs/dnsx/cmd/server@v0.1.4
```

## Quick Start

Embedded authoritative server:

```go
package main

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/dnsx/dnsserver"
	bboltstore "github.com/arcgolabs/dnsx/dnsserver/store/bbolt"
	"github.com/miekg/dns"
)

func main() {
	logger := slog.Default()
	store, err := bboltstore.Open("dnsx.db", logger)
	if err != nil {
		panic(err)
	}
	defer func() { _ = store.Close() }()

	server := dnsserver.NewServerWithRepository(
		dnsserver.Config{Listen: "127.0.0.1:5354"},
		store,
		dnsserver.WithLogger(logger),
	)

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		panic(err)
	}
	defer func() { _ = server.Stop(ctx) }()

	_, _, err = server.UpsertRRSet(ctx, "example.com", "example.com", dns.TypeNS, []dnsserver.Record{
		{TTL: 300, Data: "ns1.example.com."},
	})
	if err != nil {
		panic(err)
	}
}
```

The embeddable `dnsserver` module defaults to an in-memory `Repository` via `dnsserver.NewMemoryStore`.
Import `github.com/arcgolabs/dnsx/dnsserver/store/bbolt` only when durable bbolt-backed storage is needed.
The standalone `cmd/server` binary uses the bbolt store by default.

## Modules And Tags

This repository uses independent tags for each publishable module:

- root module: `v0.1.4`
- `dnsclient`: `dnsclient/v0.1.4`
- `dnsserver`: `dnsserver/v0.1.4`
- `dnsserver/store/bbolt`: `dnsserver/store/bbolt/v0.1.4`
- `cmd/server`: `cmd/server/v0.1.4`

For local workspace development, run:

```bash
go work sync
```

Examples live in [examples/README.md](/D:/Projects/arcgolabs/dnsx/examples/README.md:1).

# dnsx Examples

This directory contains runnable examples for the `github.com/arcgolabs/dnsx` workspace.

Run examples from the repository root:

```bash
go run ./examples/embedded-server
go run ./examples/internal-flow
go run ./examples/client-update
```

Examples included:

- `embedded-server`: start an embedded authoritative DNS server backed by bbolt.
- `internal-flow`: use `dnsserver.Server` internal client helpers to update and query itself.
- `client-update`: use `dnsclient.Client` to perform RFC2136 updates against a running `dnsserver.Server`.

# Changelog

## Unreleased

## v0.1.4 - 2026-05-10

- Split bbolt/storx persistence into the optional `github.com/arcgolabs/dnsx/dnsserver/store/bbolt` module
- Added `dnsserver.NewMemoryStore` as the default in-memory `Repository` implementation backed by `collectionx/mapping` concurrent collections
- Kept `cmd/server` wired to bbolt persistence by default for the standalone DNS process
- Upgraded arcgo dependencies including `collectionx` modules to `v0.8.0`, `dix` to `v0.10.0`, and storx modules to their latest releases
- Switched bbolt record scans to the higher-level `bboltx.Values` prefix query API

## v0.1.3 - 2026-05-07

- Added `Manager.ValidateChanges` and `Manager.PreviewChanges` for batch-change validation and dry-run previews
- Improved authoritative CNAME resolution with multi-hop chain following, loop protection, and SOA authority for terminal NODATA responses
- Added resolver additional-section population for in-zone CNAME, NS, MX, and SRV address targets
- Added typed lookup helpers for A, AAAA, CNAME, NS, MX, TXT, and SRV records in `dnsclient` and `dnsserver.Server`
- Updated release documentation for the `v0.1.3` module set

## v0.1.2 - 2026-05-05

- Upgraded `collectionx` submodules used by `dnsserver` and `cmd/server` to `v0.7.0`
- Upgraded arcgo packages that depend on `collectionx`: `configx v0.3.2`, `dix v0.7.2`, `logx v0.1.2`, and `observabilityx v0.4.0`
- Updated release documentation for the `v0.1.2` module set

## v0.1.1 - 2026-04-30

- Upgraded `dnsserver` from the old root `collectionx` package to `collectionx/set v0.6.0`
- Added `collectionx/prefix v0.6.0` and switched resolver zone matching to a trie-backed longest suffix index
- Upgraded standalone server arcgo dependencies to avoid mixed `collectionx` submodule versions
- Added zone index tests for longest-match and root-zone behavior
- Updated release documentation for the `v0.1.1` module set

## v0.1.0 - 2026-04-28

Initial public release of the `dnsx` workspace.

- Added `dnsserver` as an embeddable authoritative DNS library with:
  bbolt-backed persistence, hot-cache integration, authoritative resolution, internal client flows, and manager APIs for zone, record, RRSet, and change-set operations
- Added `dnsclient` as a reusable DNS client for query and RFC2136 update requests
- Added `cmd/server` as a standalone DNS process using `dix`, `configx`, `logx`, and standard `slog`
- Added runnable examples for embedded server startup, internal update/query flow, and external client update flow
- Added benchmark coverage for resolver queries, server queries, and dynamic update pressure
- Added zone snapshot and RRSet read models plus validation rules for SOA, apex NS, and CNAME conflicts
- Added `oops`-based domain error codes across `dnsserver`

# Changelog

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

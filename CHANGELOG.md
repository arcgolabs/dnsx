# Changelog

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

# Changelog

## v0.1.1 (2026-11-03)

- Every request logs one stderr line, `http <method> <path> -> <status> <duration>ms` (x360-common v0.5.1 `httplog`): no query, headers, cookies, tokens or bodies; `/healthz` skipped.

## v0.1.0

- Arbitration (Live service 16): Register, TimeExtend, ReportResult (XRL and UDP forms);
  reported stats forwarded to x360-xstats, first report per player row wins.

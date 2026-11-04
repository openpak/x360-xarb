# Changelog

## v0.4.0 (2026-11-04)

- Account erasure (card #120). `DELETE /internal/accounts/{account_id}/data` with `Authorization: Bearer $ACCOUNT_ERASE_KEY` on a separate listener `ERASE_ADDR` (default `:9099`, container network only, never published; not served while the key is unset). Reply `{"service":"x360-xarb","rows":N}`; one `account erased` log line per erasure. Removes the account's XUIDs from the live arbitrated sessions (registrations; a console left with no user is dropped) and the report rows already taken. New optional env `X360_XUACS_INTERNAL_URL` and `X360_XUACS_INTERNAL_KEY`; without them the endpoint is not served.

## v0.3.0 (2026-11-04)

- A result report keeps only rows of XUIDs that registered in the session (others are dropped and counted in the log), and forwards the participant list to x360-xstats with `X360_XARB_ARB_KEY` so xstats accepts them (xstats v0.8.0, #101). `PostTo` takes the key as a new argument.

## v0.2.0 (2026-11-04)

- Arbitrated sessions (registrations, expiry, rows already taken from a report) live in Postgres (`X360_XARB_DATABASE_URL`, schema `x360_xarb`, required) instead of memory: a restart loses nothing.
- Stats of a result report still go to x360-xstats (`X360_XARB_STATPOST_URL`), which writes them to the leaderboards core; xarb holds no score data.
- Logging: no request bodies or console accounts in the log; the request line comes from httplog.

## v0.1.1 (2026-11-03)

- Every request logs one stderr line, `http <method> <path> -> <status> <duration>ms` (x360-common v0.5.1 `httplog`): no query, headers, cookies, tokens or bodies; `/healthz` skipped.

## v0.1.0

- Arbitration (Live service 16): Register, TimeExtend, ReportResult (XRL and UDP forms);
  reported stats forwarded to x360-xstats, first report per player row wins.

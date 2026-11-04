# Deploy

Primary host, quadlet `deploy/openpak-x360-xarb.container` (IP pinned in openpak.network),
Traefik routes from `deploy/traefik.yml` merged into the shared dynamic file.
`X360_XARB_STATPOST_URL` points at x360-xstats inside openpak.network. The console reaches the
service only once 16 is in the KDC's D7 list (`X360_KDC_D7_SERVICES`) and the gateway serves
UDP (x360-gateway v0.9.6); re-sign-in after changing the list, since it comes from the
boot-time ticket.

`X360_XARB_DATABASE_URL` (Postgres, schema `x360_xarb` in database `x360`; required) comes from `~/openpak/deploy/x360-xarb/db.env`, mode 0600, never committed.

`X360_XARB_ARB_KEY` (shared with x360-xstats as `X360_XSTATS_ARB_KEY`, from `~/openpak/deploy/x360-arb-key.env`, mode 0600, never committed) lets xarb tell xstats which XUIDs took part in a result report. xarb drops result rows for XUIDs that never registered in the session; xstats accepts those participants only with the key, and otherwise only the reporter's own XUIDs.

## Account erasure

`DELETE /internal/accounts/{account_id}/data` with `Authorization: Bearer $ACCOUNT_ERASE_KEY` on a separate listener `ERASE_ADDR` (default `:9099`, container network only, never published; not served while the key is unset). Reply `{"service":"x360-xarb","rows":N}`; one `account erased` log line per erasure.

Website step name: `x360-xarb`.

Removes the account's XUIDs from the `arbs` document: user entries of registrations (a registrant left with no users is dropped with its console id) and `written` rows. Sessions expire by themselves half an hour after the match. Needs `X360_XUACS_INTERNAL_URL`/`_KEY` (new). Run before x360-xuacs.

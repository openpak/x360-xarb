# Deploy

Primary host, quadlet `deploy/openpak-x360-xarb.container` (IP pinned in openpak.network),
Traefik routes from `deploy/traefik.yml` merged into the shared dynamic file.
`X360_XARB_STATPOST_URL` points at x360-xstats inside openpak.network. The console reaches the
service only once 16 is in the KDC's D7 list (`X360_KDC_D7_SERVICES`) and the gateway serves
UDP (x360-gateway v0.9.6); re-sign-in after changing the list, since it comes from the
boot-time ticket.

`X360_XARB_DATABASE_URL` (Postgres, schema `x360_xarb` in database `x360`; required) comes from `~/openpak/deploy/x360-xarb/db.env`, mode 0600, never committed.

`X360_XARB_ARB_KEY` (shared with x360-xstats as `X360_XSTATS_ARB_KEY`, from `~/openpak/deploy/x360-arb-key.env`, mode 0600, never committed) lets xarb tell xstats which XUIDs took part in a result report. xarb drops result rows for XUIDs that never registered in the session; xstats accepts those participants only with the key, and otherwise only the reporter's own XUIDs.

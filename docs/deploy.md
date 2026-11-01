# Deploy

Primary host, quadlet `deploy/openpak-x360-xarb.container` (IP pinned in openpak.network),
Traefik routes from `deploy/traefik.yml` merged into the shared dynamic file.
`X360_XARB_STATPOST_URL` points at x360-xstats inside openpak.network. The console reaches the
service only once 16 is in the KDC's D7 list (`X360_KDC_D7_SERVICES`) and the gateway serves
UDP (x360-gateway v0.9.6); re-sign-in after changing the list, since it comes from the
boot-time ticket.

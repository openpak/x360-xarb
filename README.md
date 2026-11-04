# Xbox 360 Arbitration Server

OpenPak's Xbox 360 arbitration server (Live service 16, `XArb`).

| | |
| --- | --- |
| Network | Xbox 360 |
| Role | Ranked-session arbitration (`XSessionArbitrationRegister`), result reports and time extensions |
| Depends on | [`x360-common`](https://github.com/openpak/x360-common) (shared Go library, vendored); [`x360-xstats`](https://github.com/openpak/x360-xstats) for reported stats |
| Image | `ghcr.io/openpak/x360-xarb` |
| Used by | XAM through the security gateway (HTTP over the tunnel's TCP, and HTTP in one UDP datagram) |
| Licence | AGPL-3.0-only |

## What it serves

| Path | XRL | What |
| --- | --- | --- |
| `POST /XArb/Register.ashx` | 0xF00 | Register this console in an arbitrated session; reply lists every registrant so far |
| `POST /XArb/TimeExtend.ashx` | 0xF01 / UDP | Extend the session's lifetime |
| `POST /XArb/ReportResult.ashx` | 0xF02 / UDP | Report the match result; its stats go to xstats |
| `GET /healthz` | | Health probe |

Every request needs the gateway's console account header (`X-X360-Account`, trusted only with
`X360_GATEWAY_KEY`). A registrant's machine id is the console's KDC machine account id, its users
are the users signed in on the tunnel. Only registrants may extend or report. Every console of a
ranked match reports; the first report of a player's row in a view wins and later copies are
dropped, so summed columns count once. Registrations and the rows already taken live in Postgres (`X360_XARB_DATABASE_URL`, schema `x360_xarb`, required), so a restart loses nothing. The stats themselves go to [`x360-xstats`](https://github.com/openpak/x360-xstats), the one place that turns statpost writes into entries of the leaderboards core; xarb keeps no score table.

## Run

```sh
X360_XARB_STATPOST_URL=http://localhost:10007/xstats/statpost.ashx GOWORK=off go run -mod=vendor ./cmd/x360-xarb
```

## Docs

- [Protocol](docs/protocol.md)
- [Deploy](docs/deploy.md)

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).

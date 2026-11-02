# Protocol: x360-xarb

Live service 16 (`XArb`), XAM 17559, static RE. Default port 1000 (the D7 port rule:
port = service id, except 16 -> 1000). Bodies are XRL "xon" (little-endian fields, no padding)
or, over UDP, the same fields written by hand.

## Common head (24 bytes)

`u16 version (1) | u16 flags | u32 title id | u64 session XNKID | u64 arbitration nonce`

## 0xF00 Register (`FUN_8182d4e0`, called by `FUN_8191ac28`)

Request: head with flags 6, then `u16` session lifetime in seconds. XAM sets its own expiry to
now + lifetime and sends TimeExtend 60 s before it.

Reply: `u16 n`, n x `{u64 machine id, u8 trustworthiness, u8 m, m x {u64 xuid, u32}}`. XAM
copies each registrant and keeps only the XUIDs that are members of its session
(`FUN_81918f50`). OpenPak sends machine id `0x0009 << 48 | console digits` (the KDC machine
account id), trust 1, the tunnel's signed-in users, the `u32` as 0, in registration order.

## TimeExtend (`FUN_81919c20`)

Sent over UDP (below): head, then `u16 300`. XRL 0xF01 has the same fields and no reply body.

## 0xF02 ReportResult (`FUN_8182d6f0`, called by `FUN_81919d40`)

XRL (xon/5): head with flags `0x100` (`| 0x4000` when the session's flag bit 0 is set), then the
stats block `u32, u32, u16 n, n procs`. The schema has four optional blocks counted by
arguments that are not on the wire; the only caller sets the stats block alone.

If the XRL post fails with an error outside facility 0x8015, or the session has flag 0x1000,
XAM sends the UDP form (xon/16): head with flags `0x100 >> 8` (`| 0x40`), then `u32` byte
length and the procs. The flag drops when the stats do not fit the 0x4F0 byte datagram.

Procs are statpost procs (`{u16 proc, u16 m, m x {u8 type, value}}`, see x360-xstats): column
procs carry XUID, view, `u16 0`, then column/value pairs. OpenPak forwards them to xstats as a
statpost (0x705: `u32 title, u16 n, procs`) with the reporting console's headers.

## UDP form (`FUN_81917ab0`)

One datagram to the service address and port: a whole HTTP request,
`POST <path> HTTP/1.0`, `User-Agent: 16/1.00.17559`, `Content-type: xon/16`, `Content-Length`,
then the body. Source port is random above 5000; 3 tries, 1 s each. XAM reads the reply as text:
success needs `200 OK` in it, and `X-Err: <hex>` sets the HRESULT (`FUN_81918858`). The
security gateway turns the datagram into an HTTP request to this service and sends the HTTP
response back as one datagram (x360-gateway docs/protocol.md, "UDP").

## Derivation

Schemas of 0xF00-0xF02 were dumped from the 17559 XRL table, with argument order matched to the XAM builders and callers. The Register reply built by the tests decodes field for field under the XAM codec model. The UDP bodies and the text reply check come from the XAM decompile.

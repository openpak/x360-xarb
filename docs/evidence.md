# Evidence

- 2026-11-01, static: schemas of 0xF00-0xF02 dumped from the 17559 XRL table; argument order
  matched to the XAM builders and callers (protocol.md). The Register reply built by this
  service's tests decodes field for field under the XAM codec model. The UDP bodies and the
  text reply check come from the XAM decompile.
- Not yet seen from a console: no title has sent service 16 traffic to OpenPak. Unverified: the
  meaning of the two `u32` in the XRL stats block, the per-user `u32` in the Register reply,
  and whether any title compares registrant machine ids with something of its own.

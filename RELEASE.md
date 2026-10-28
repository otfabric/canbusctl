# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.2.0

**Date:** 2026-10-28
**Previous release:** v0.1.0

canbusctl on go-canbus v0.2.0: Vector trace formats, AUTOSAR databases, a
fault bridge and tested interoperability.

- Traces: `record`, `replay`, `monitor -o/--output`, `dbc decode --trace` and
  `inject --trace` read and write Vector ASC and BLF besides candump, JSON
  Lines and pcap (pcapng is read). New `trace convert <in> <out>` (filters,
  `--no-errors`, `--max-frames`; reports what the target format cannot keep)
  and `trace info` (kinds, duration, per-identifier counts and rates). ASC and
  BLF times are zone-less local time: set `TZ` to match the recording.
- Databases: ARXML (the CAN part of AUTOSAR 4 system descriptions) wherever a
  database is read; `dbc convert --to kcd` writes KCD and prints what KCD
  cannot hold. Conversions: DBC, KCD, ARXML → DBC, KCD, JSON.
- `bridge -a <if> -b <if>`: forwards frames between two interfaces (vcan,
  vxcan, `virtual`; others with `--yes`) through seeded faults — `--drop`,
  `--duplicate`, `--delay-probability`/`--delay`, `--reorder`, `--corrupt`,
  `--filter` — for `--duration`, with per-direction counters (text or JSON).
- `inject` runs on the same chaos bridge (go-canbus `chaos`): new `--reorder`;
  JSON `faults` gains `received`, `forwarded`, `reordered`. Other flags and
  output are unchanged.
- ISO-TP: `--fc-delay` (N_Br) on `isotp receive`/`send` delays each flow
  control. Against the kernel's ISO-TP sender on vcan a fast userspace
  receiver needs about 200 µs: the kernel drops a flow control that beats the
  echo of its own last block frame. `isotp send --kernel` no longer resends a
  whole message when the Go runtime interrupts the kernel's wait for the
  transmission (fixed in go-canbus v0.2.0). `isotp monitor` is listen-only.
- A socket deadline that fires a moment before the command's deadline ends a
  `--timeout`/`--duration` run normally instead of failing with a timeout or
  I/O error (`isotp receive --kernel -n 0`, `bridge`).
- Tests: end-to-end tests of the built binary (every trace format, vcan
  pipelines, ISO-TP between processes and with the kernel, bridge
  processes); chaos tests (ISO-TP through a faulty bridge never delivers
  wrong data, every process returns by its deadline); interop tests against
  canbus-interop (can-utils/kernel, python-can, python-can-isotp, cantools,
  canmatrix); vcan tests on vcan0 and vcan1 in CI with kernel ISO-TP;
  microVM runs on kernels 6.1–6.18. Coverage 94.3 %.

---

## v0.1.0

**Date:** 2026-10-27
**Previous release:** none

First release of canbusctl, the command-line toolkit of the OT Fabric CAN
ecosystem, built on go-canbus v0.1.0.

- Interfaces (Linux): `interfaces`, `info`, `configure` (bit rates, CAN FD,
  controller modes, restart, MTU), `up`, `down`.
- Frames: `monitor` (filters, live DBC decoding), `send`, `send-fd`, `record`
  (candump, JSON Lines, pcap), `replay` (original, scaled or maximum speed,
  filters, limits), `stats`.
- Databases: `dbc inspect`, `validate`, `decode`, `encode`, `generate` (typed
  Go and JSON descriptors), `convert` (DBC/KCD to DBC or JSON).
- ISO-TP: `isotp send`, `receive` (userspace or `--kernel`), `monitor`
  (listen-only).
- Simulation: `simulate` (deterministic traffic from a database), `inject`
  (seeded faults, error and malformed frames on the virtual bus).
- Safety: passive by default, `--yes` for transmissions on non-virtual
  interfaces and for administrative changes, bounded transmissions.
- Output: text, table, versioned JSON; stable exit codes.

---

# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

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

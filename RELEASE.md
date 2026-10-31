# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.7.2

**Date:** 2026-10-31
**Previous release:** v0.7.1

### Changed

- Policy 1.2: build output in `bin/` and `release/`, a per-package coverage floor, and more tests and CI. No change in behaviour.
- Built on go-canbus v0.7.2, go-serial v0.2.1.

---

## v0.7.1

**Date:** 2026-10-30
**Previous release:** v0.7.0

### Dependencies

- `go-canbus` → v0.7.1 with `go-serial` v0.2.0: on macOS and BSD the baud constant no longer ends up in `Cflag`, where it set parity and stop-bit flags on the port (slcan).

---

## v0.7.0

**Date:** 2026-10-30
**Previous release:** v0.6.0

Exit codes converge on the organisation table (ADR-0006). **Scripts that test for these codes must change.**

| Case | Old | New |
|---|---|---|
| Missing privilege (`permission`, CAP_NET_ADMIN) | 7 | 9 |
| Interrupted before a result (`code` `canceled`) | 10 (`internal`) | 130 (`interrupted`) |

All other codes are unchanged. `error-v1.schema.json` adds the kind `interrupted`. Exit 7 is now reserved for a partial result, which canbusctl does not produce today.

`CANBUSCTL_OUTPUT_FORMAT` is now accepted besides `CANBUSCTL_FORMAT`, which is the canonical name and wins when both are set.

---

## v0.6.0

**Date:** 2026-10-29
**Previous release:** v0.5.0

## Summary

CAN XL in recordings, replays and trace conversion, on the virtual bus,
and pcapng output. Built on go-canbus v0.7.0.

## Additions

- **`record --xl`** records CAN XL frames with CAN and CAN FD frames, in
  arrival order, to candump logs, JSON Lines, pcap or pcapng (other
  formats are refused before anything is opened).
- **`replay --xl`** sends a trace's CAN XL frames too; without it they are
  counted (`skippedXL`) and reported, not sent. `--print` shows them.
- **`trace info`** counts CAN XL frames (`xl`; identifiers
  `XL <vcid><priority>`).
- **pcapng output** wherever traces are written (`record`, `trace
  convert`, `monitor -o`, `xl monitor -o`), with interface names.
- **`xl send` and `xl monitor` on `-i virtual`**; `xl monitor --output`
  takes .pcap and .pcapng.
- Filters (`--filter`) select CAN XL frames by priority, as the kernel's
  filters do.

## Fixes

- **`trace convert` dropped CAN XL frames silently.** Its reader skipped
  them, against the command's rule that nothing is dropped unreported.
  CAN XL frames are now kept in candump, JSON Lines, pcap and pcapng and
  reported as lost for ASC, BLF and TRC.

## Verification

- On vcanxl0 (Linux 7.0 and the CI kernels): `record --xl` to pcapng of
  CAN XL frames up to 2048 bytes and a CAN frame, `trace info` of the
  file, and `replay --xl` of it received in order by `xl monitor`.
- On the virtual bus: record, monitor, replay and convert of mixed CAN,
  CAN FD and CAN XL traffic, on every OS.

---

## v0.5.0

**Date:** 2026-10-28
**Previous release:** v0.4.0

## Summary

CAN XL frames on SocketCAN. Built on go-canbus v0.5.0.

## Additions

- **`canbusctl xl send -i <iface> <frame>...`**: CAN XL frames in can-utils
  notation, `<vcid><prio>#<flags>:<sdt>:<af>#<data>` (for example
  `00242#80:03:00C0FFEE#11223344`), with `--repeat`, `--gap`, `--rate`,
  `--max-frames` and `--duration`; like every transmitting command it needs
  `--yes` on an interface that is not vcan or vxcan.
- **`canbusctl xl monitor -i <iface>`**: every frame of a CAN XL interface,
  CAN XL, CAN and CAN FD, in arrival order, as text or JSON (`"kind":"xl"`
  records with priority, VCID, SEC, RRS, SDT, AF and data; the summary
  counts the CAN XL frames); `--output` writes candump logs or JSON Lines,
  the trace formats that carry CAN XL. Passive.
- The interface needs the CAN XL MTU (`ip link set vcanxl0 mtu 2060`, Linux
  ≥ 6.2); the virtual bus and slcan adapters have no CAN XL (exit 8).

## Verification

- End to end on `vcanxl0` (CI and the microVMs on kernels ≥ 6.2): one
  process sends CAN XL frames (up to 600 bytes, VCID, SEC/RRS) and a CAN
  frame, another monitors them and writes candump and JSON Lines traces.
- Against can-utils v2025.01 (canbus-interop v0.5.0, contract 1.3):
  `xl send` → candump and cansend → `xl monitor --format json` with the six
  frames of `xl.jsonl` (1 to 2048 bytes, VCIDs 0x42 and 0xFF).
- JSON output checked against schemas/record-v1.schema.json (which now
  describes CAN XL frame records).
- Interop against canbus-interop v0.5.0 (amd64 and arm64): 88 passed,
  0 failed, 6 skipped (the CAN XL scenarios with python-can, socketcan-rs
  and Scapy, which have no CAN XL), 76 known-difference records
  (unchanged).
- Coverage 94.1 % (Linux with vcan, the CI gate is 90 %).

---

## v0.4.0

**Date:** 2026-10-28
**Previous release:** v0.3.0

## Summary

Error frames reach `monitor --errors` and `record --errors` again, slcan
adapters report their status, and ARXML databases may use multiplexed
PDUs. Built on go-canbus v0.4.0.

## Fixes

- **`--errors` showed no error frames.** `monitor` and `record` asked the
  interface for error frames but subscribed without them, so they were
  dropped before display on every interface type. New tests inject
  error frames on the virtual bus and fail without the fix.

## Additions

- **slcan adapter status.** "e" (bus error) and "s" (controller state,
  error counters) records from an adapter become error frames, decoded as
  the Linux slcan driver decodes them (go-canbus v0.4.0): `monitor -i
  slcan:… --errors` shows ACK errors, warning, passive and bus-off.
  Filters on slcan now run inside go-canbus (`Conn.SetFilters`).
- **Multiplexed ARXML PDUs** in `--dbc` databases and `db` commands:
  MULTIPLEXED-I-PDUs read as cantools reads them (selector
  `<frame>_selector1`).

## Verification

- Interop against canbus-interop v0.4.0 (amd64 and arm64): 86 passed,
  0 failed, 0 skipped, 76 known-difference records. The rise from 27 known
  is the new multiplexed ARXML fixture: cantools rejects selector values
  no alternative describes, canmatrix 1.3.3 reads only the selector.
- Coverage 94.4 % (Linux with vcan, the CI gate is 90 %).

---

## v0.3.0

**Date:** 2026-10-28
**Previous release:** v0.2.2

canbusctl on go-canbus v0.3.0: serial-line CAN adapters on every OS and
PEAK TRC traces.

- Serial-line CAN (slcan, LAWICEL ASCII) adapters — CANable, CANtact,
  USBtin, LAWICEL CANUSB — on Linux, macOS and Windows without a kernel
  driver: `--interface slcan:/dev/ttyACM0` (or `slcan:COM3`), and
  `slcan:tcp://host:port` for an adapter behind a serial server. Every
  command that takes an interface accepts it, including `bridge` (a
  SocketCAN interface ↔ an adapter). New global flags `--slcan-bitrate`
  (default 500000; 0 keeps the adapter's), `--slcan-data-bitrate` (CAN FD
  data phase: 2000000 or 5000000, CANable 2.0) and `--slcan-baud`.
- Safety: `monitor`, `record`, `stats` and `isotp monitor` open an adapter
  listen-only ("L"): it neither transmits nor acknowledges frames.
  Transmitting on an slcan adapter always needs `--yes` (exit 9).
- slcan has no error frames, error state indicator, echo or hardware
  filters; `--filter` is applied in software.
- PEAK TRC (`.trc`, versions 1.0 to 2.1 read, 2.1 written) wherever traces
  are read or written: `record`, `replay`, `monitor -o`, `trace convert`
  and `trace info`, `dbc decode --trace`, `inject --trace`. `trace convert`
  reports error classes TRC cannot keep (it keeps protocol violations and
  error counters).
- Tests: an end-to-end test drives canbusctl against the Linux kernel's
  slcan driver (two linked ptys, the driver on one, canbusctl on the
  other; both directions, recorded to TRC) in the Kernels microVMs; command
  tests against a TCP adapter emulation check the setup commands
  (bit rates, listen-only), `--yes` and software filters.

---

## v0.2.2

**Date:** 2026-10-28
**Previous release:** v0.2.1

canbusctl on go-canbus v0.2.2.

- `monitor`, `record`, `stats` and the ISO-TP commands could lose the
  first frames that arrived between opening the interface and subscribing
  to it (go-canbus's Bus started reading at once). They now open the bus
  with `canbus.WithStartOnSubscribe`, so frames wait in the socket until
  the command listens. Found by the end-to-end tests in a slow microVM,
  where `record` kept 21 of 60 frames.
- Tests: the end-to-end harness waits until the kernel has registered a
  started process's CAN filters (/proc/net/can) before sending to it, and
  prints the recording when `record` misses frames.

---

## v0.2.1

**Date:** 2026-10-28
**Previous release:** v0.2.0

canbusctl on go-canbus v0.2.1. go-canbus now reports a socket deadline that
fires a moment before the command's own deadline as that deadline, and
`chaos.Bridge.Run` returns nil when its context ends during a send; the
workarounds canbusctl v0.2.0 carried for both are removed. Commands behave as
in v0.2.0.

---

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
  microVM runs on kernels 6.1–6.18. Coverage 94.2 %; interop with
  canbus-interop v0.2.0: 63 pass, 27 known differences, no skips.

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

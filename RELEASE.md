# Releases

Releases are made by the Release workflow (Actions → Release → Run workflow,
with the version bump). It computes the tag, builds the binaries for linux
(amd64, arm64, armv7), darwin (amd64, arm64) and windows (amd64, arm64) and
publishes the GitHub release with the section of this file that matches the
version as its notes. Add the section before running it.

## v0.11.2

**Date:** 2026-11-13
**Previous release:** v0.11.1

Interop pin update; no change to canbusctl's own code.

- The interop suite pins the canbus-interop v0.7.0 images by digest (was
  v0.6.0), the same as go-canbus v0.9.1. Contract 1.4 adds only the
  version document's `independentEngine` field and no scenario.
- Interop against canbus-interop v0.7.0 (CI, amd64 and arm64): 88 passed,
  0 failed, 6 skipped, 76 known-difference records (unchanged from
  v0.6.0).

---

## v0.11.1

**Date:** 2026-11-13
**Previous release:** v0.11.0

Dependency update; no change to canbusctl's own code.

- go-canbus v0.9.1 (from v0.9.0) and go-serial v0.3.0 (from v0.2.1).
  go-serial now refuses out-of-range serial parameters (data and stop
  bits, RS-485 RTS delays above `serial.MaxRS485Delay`) instead of
  wrapping them; this reaches the slcan transport (`-i slcan:<device>`).
- `version` reports go-canbus 0.9.1.

---

## v0.11.0

**Date:** 2026-11-08
**Previous release:** v0.10.0

Built on go-canbus v0.9.0, which go-canbus tests against CAN logs recorded on
twelve real vehicles. Two of its fixes reach canbusctl's trace output.

- **Channel numbers in ASC, BLF and TRC changed.** `trace convert`, `record`
  and `monitor --output` used to write every interface that is not a bare
  number as channel 1, so a recording of several buses collapsed into one
  channel. Now `can0` is channel 1, `can4` channel 5, `vcan2` channel 3; a bare
  number stays itself, any other name is 1. Scripts that read channel 1 for
  `can1` frames must read channel 2. `trace convert` still reports each
  interface name it turns into a number (now also `0`, written as 1).
- **A BLF channel above 65535 is refused** (exit 3, `unrepresentable_frame`)
  instead of being written as channel 1 and reported as a loss.
- **`--direction`** (new flag of `trace convert`, `record` and `monitor`): a
  candump output ends every line with `R` or `T` (sent by the logging node),
  as `candump -L -x` writes it. A log with tokens now converts back to
  itself; without the flag `trace convert` still reports the lost `T` marks.
  With an output that is not candump it is an error (exit 3).
- `version` reports go-canbus 0.9.0.

---

## v0.10.0

**Date:** 2026-11-07
**Previous release:** v0.9.0

`version` prints one line in the format shared by every OT Fabric CLI, and
`version --json` one JSON object; shell completion covers every flag.

- `version`: `canbusctl 0.10.0 (tag v0.10.0, commit <full sha>, built
  <RFC 3339 UTC>, go-canbus 0.8.0, go-serial 0.2.1, go1.26.x, linux/amd64)`:
  the OT Fabric libraries the binary is built on come after the build date. A development
  build (`go build`, `go run`, `make build` off a tag) reports `dev`, the VCS
  commit with `-dirty` for local changes (`go build`) and `built unknown`;
  `go install …@v0.10.0` reports `0.10.0`.
- `version --json` (new flag; also `--format json` and
  `CANBUSCTL_FORMAT=json`): `{"name","version","tag","commit","built",
  "libraries","go","platform"}` on one line, every key always present;
  `libraries` is
  `[{"name":"go-canbus","version":"0.8.0"},{"name":"go-serial","version":"0.2.1"}]`.
- **The version output changed.** The text form was three lines; the JSON
  form was the `schemaVersion`/`command`/`result` envelope with `version`,
  `tag`, `commit`, `buildDate`, `goCanbus`, `go`, `platform`. Scripts that
  parse `canbusctl version` must change: `buildDate` is now `built`,
  `goCanbus` is now the `libraries` entry `go-canbus` (version without the
  leading `v`), and there is no envelope. No other command's output changes.
- Shell completion (`completion bash|zsh|fish|powershell`) completes the
  values of every flag with a fixed set (`--format`, `--slcan-bitrate`,
  `--slcan-data-bitrate`, `--mtu`, `--dlc`, `--tx-dl`, `--file-format`,
  `--addressing`, `--pattern`, `--error-frame`, `dbc convert --to`), database
  files (`.dbc`, `.kcd`, `.arxml`) for `--dbc` and the `dbc` commands, trace
  files by extension for `--trace`, `--output`, `replay`, `trace convert` and
  `trace info`, and no file names for free values (interfaces, identifiers,
  numbers, durations).
- `make build` injects the full commit SHA; the version and tag only when
  HEAD is exactly on a tag (without the leading `v` in the version, as the
  Release workflow does), so a build off a tag reports `dev`.

---

## v0.9.0

**Date:** 2026-11-07
**Previous release:** v0.8.0

A safety refusal gets its own exit code, 13, as in the organisation table
(otfabric/engineering `standards/api-design.md#cli-exit-codes`); 9 now means
a missing privilege only. **Scripts that test for the safety exit code must
change.**

| Case | Old | New |
|---|---|---|
| Refused for safety without `--yes` (`safety`, `confirmation_required`): a transmission on an interface that is not virtual, or an administrative change | 9 | 13 |

All other codes are unchanged; `permission` (CAP_NET_ADMIN) stays 9. The
JSON error document is unchanged: `kind` `safety` and `code`
`confirmation_required` as before.

---

## v0.8.0

**Date:** 2026-11-04
**Previous release:** v0.7.2

## Summary

Built on go-canbus v0.8.0 (SAST hardening). SAST review of canbusctl at
severity and confidence low: 10 findings, 9 fixed in code, 1 recorded as
a false positive (the seeded generator of `simulate --pattern random`).
New fuzz, hostile-input, chaos and signal tests found 14 more defects,
fixed here. Go 1.26 is the minimum. Interoperability against
canbus-interop v0.6.0: 88 passed, 0 failed, 6 skipped, 76 known-difference
records (unchanged).

### Changed

- **Go 1.26 is the minimum** (was 1.23); CI tests 1.26 and 1.27.
- **Files canbusctl creates are owner-only (0600)**: `record -o`, `monitor -o`, `xl monitor -o`, `trace convert`, `dbc generate`/`convert` output. Existing files keep their mode.
- **Exit 3 instead of a silent default or a wrap:**
  - `send --rtr --dlc` outside 0..8 (`--dlc 260` sent DLC 4).
  - negative, NaN or infinite `--rate`, `--max-frames`, `--duration` (they meant "no limit"); a `--repeat` whose total overflows (it bypassed `--max-frames`).
  - `inject --malformed` beyond 65536 or beyond `--max-frames`; NaN fault probabilities in `inject` and `bridge`.
  - `inject --trace` with a malformed record (it injected part of the trace and exited 0).
  - `trace convert` onto its own input (it truncated the input).
  - `dbc generate --package` with a Go keyword; a DBC named after one now gets `<name>db`.
  - a database layout go-canbus rejects (`candb.ErrLayout`, was exit 10 `internal_unexpected`); `dbc decode` stdin lines over 64 KiB (was exit 10).
- `isotp receive` stopped by SIGINT or SIGTERM exits 130 (`canceled`) as docs/exitcodes.md states; it exited 5.
- `isotp send --size` beyond `--max-size` is refused before allocating (exit 6, as before).
- A tiny `--rate` (such as `1e-300`) is clamped instead of becoming unlimited on amd64.
- `trace convert` removes its partial output file when it fails (only a regular file it created; never a link target or device). A frame the target format cannot hold (go-canbus `ErrUnrepresentable`, such as pcap before 1970 or a BLF CAN FD channel over 255) exits 3 with the new code `unrepresentable_frame`, the record number and the reason; BLF channel loss is listed under `losses`.
- **Output paths that are symbolic links are refused** (exit 3, `file_open_failed`) by every command that writes a file; they used to be followed and their target truncated. Unix also opens outputs with O_NOFOLLOW.
- `replay` reads a whole trace file before the first frame: a malformed record anywhere exits 3 with nothing sent (it exited 10 `internal_unexpected` after sending the frames before it). From standard input a malformed record after frames were sent exits 10 with code `invalid_capture`.
- go-canbus capture parse errors are classified `invalid_capture` (exit 3) wherever they surface.

### Tests

13 new fuzz targets (frame, ID, hex, XL notation, fault specs, send limits, slcan flags, package names, and every read-only command run on fuzzed files), chaos seed sweeps of `bridge` and `inject` (virtual bus and vcan, with leak checks), hostile databases and traces, `--yes` checked for every transmitting command, SIGINT/SIGTERM end to end.

### Dependencies

- go-canbus v0.8.0; canbus-interop v0.6.0 images (by digest); shared workflows v3.3.1.

---

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

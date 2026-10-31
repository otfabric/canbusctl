# Testing

| Command | What |
|---------|------|
| `make test` | All tests with the race detector; Linux interface tests skip without vcan0 |
| `make test-vcan` | The same on Linux, one package at a time, failing (not skipping) when vcan0 or vcan1 is missing |
| `make coverage` | Whole-module coverage (`-coverpkg=./...`), gated at 90 % |
| `make coverage-pkg-check` | Every package at least `COVER_PKG_MIN` (90 %); lowest measured in CI: 93.7 % (cmd). The vcan job runs it |
| `make e2e` | End-to-end tests of the built binary (Linux parts need vcan0, vcan1) |
| `make interop` | The binary against the canbus-interop images (Docker, vcan0) |
| `make fuzz` | Fuzz targets of the structured inputs: frame notation, `signal=value`, filters, trace format, database kind |
| `make cross` | vet for every release platform, linux/arm (32-bit) included |
| `make check` | fmt, staticcheck, golangci-lint, vet, cross, govulncheck, test, coverage, coverage-pkg-check |

Per-package override: the root package main is not gated (`COVER_PKG_SKIP`). It only calls `cmd.Execute`; the e2e suite runs the built binary, which `go test` coverage does not instrument.

**In-process command tests** (`cmd/*_test.go`) run the real command tree
through `cmd.Run` with captured streams and check exit codes, text and JSON.
Commands that receive run on the in-process virtual bus while the test sends
frames on it; ISO-TP sender and receiver run as two concurrent invocations.
Golden files (`cmd/testdata/golden`, regenerate with
`go test ./cmd -update`) pin `dbc inspect` and `dbc decode` output.

**vcan tests** (`cmd/*_linux_test.go`) exercise interfaces, info, stats,
monitor (with `-o`), record, send, send-fd, replay, simulate, inject and
bridge (vcan0 ↔ virtual, vcan0 ↔ vcan1 under faults) on real vcan
interfaces, and ISO-TP between the userspace stack and the kernel in both
directions (`--kernel`, `--fc-delay`). They need

```sh
sudo modprobe vcan; sudo modprobe can-isotp
for i in vcan0 vcan1; do sudo ip link add $i type vcan && sudo ip link set $i mtu 72 up; done
```

Packages share the interfaces: run them with `go test -p 1` (the make
targets do).

**End-to-end tests** (`e2e/`) build the binary and drive it as separate
processes: a simulated trace through every format (candump → JSON Lines →
pcap → ASC → BLF → candump …) decoding identically, KCD round trips; on
Linux, replay → record → convert → replay → monitor on vcan for every
format, ISO-TP between two canbusctl processes and against the kernel, a
bridge process between vcan0 and vcan1.

**Chaos tests** (`e2e`, `TestChaosISOTPThroughBridge`): ISO-TP through a
bridge process that drops, duplicates and delays frames, over several seeds,
never delivers wrong data (the receiver reports the exact message or an
error), some messages get through, and with every fault (reorder and corrupt
too) all processes return by their deadlines.

**Interop tests** (`interop/`, build tag `interop`) run the binary against
the pinned canbus-interop images: `send`/`send-fd` → can-utils, python-can,
socketcan-rs and Scapy receivers; their senders → `monitor --format json`; `isotp send`/`receive`
↔ the kernel (can-utils) and python-can-isotp, and `--kernel` →
python-can-isotp; `dbc decode`/`encode` against cantools and canmatrix (DBC
and ARXML fixtures); `dbc convert --to kcd` read back by canmatrix; `record`
.asc/.blf/.trc/.log read by python-can (`log-convert`). A capability an image
lacks is a recorded skip, never a pass; deliberate differences are recorded
as known differences (`CANBUS_INTEROP_REPORT` writes the matrix).

**Kernels workflow**: the vcan, kernel ISO-TP, bridge, end-to-end and chaos
tests also run in virtme-ng microVMs booted with the Ubuntu generic kernel
and mainline 6.1, 6.6, 6.12 and 6.18 (`scripts/microvm-test.sh`).

`CANBUS_REQUIRE_VCAN=1` turns a missing interface into a failure;
`CANBUS_REQUIRE_ISOTP=1` does the same for the can-isotp module, which
GitHub's hosted runners lack (the kernel ISO-TP test skips there and runs in
local Linux VMs).

**At the time of v0.3.0** (same VM, canbus-interop v0.3.0 images): interop
84 pass, 27 known differences, no skips (`record` .trc read by python-can
added); the end-to-end test against the kernel's slcan driver
(`TestSlcanKernel`, root, socat) passes, as in the Kernels microVMs;
whole-module coverage 92.4 % (macOS).

**Interop at the time of v0.2.2** (same VM, canbus-interop v0.2.0 images,
socketcan-rs and Scapy added): 83 pass, 27 known differences, no skips.

**Results at the time of v0.2.0** (Linux arm64 VM, kernel 7.0, vcan0, vcan1
and can-isotp): all tests pass with `-race`; whole-module coverage 94.2 %.
Interop with the pinned canbus-interop v0.2.0 images (contract 1.1): 63 pass,
27 known differences, no skips (with the v0.1.0 images, which lack ARXML and
log-convert: 54 pass, 19 known differences, 9 skips).

At v0.1.0: whole-module coverage 94.5 % (macOS, without the vcan tests:
94.4 %).

**Not tested here:** real CAN controllers (bit timing changes, bus-off, error
counters) — `configure` with real hardware is covered by the request encoding
tests in go-canbus only.

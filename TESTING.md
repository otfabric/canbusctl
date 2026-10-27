# Testing

| Command | What |
|---------|------|
| `make test` | All tests with the race detector; Linux interface tests skip without vcan0 |
| `make test-vcan` | The same on Linux, failing (not skipping) when vcan0 is missing |
| `make coverage` | Whole-module coverage (`-coverpkg=./...`), gated at 90 % |
| `make fuzz` | Fuzz targets of the structured inputs: frame notation, `signal=value`, filters |
| `make cross` | vet for every release platform, linux/arm (32-bit) included |
| `make check` | fmt, staticcheck, golangci-lint, vet, cross, govulncheck, test, coverage |

**In-process command tests** (`cmd/*_test.go`) run the real command tree
through `cmd.Run` with captured streams and check exit codes, text and JSON.
Commands that receive run on the in-process virtual bus while the test sends
frames on it; ISO-TP sender and receiver run as two concurrent invocations.
Golden files (`cmd/testdata/golden`, regenerate with
`go test ./cmd -update`) pin `dbc inspect` and `dbc decode` output.

**vcan tests** (`cmd/vcan_linux_test.go`) exercise interfaces, info, stats,
monitor, record, send, send-fd, replay, simulate and inject on a real vcan
interface, and an ISO-TP transfer between the userspace sender and a kernel
ISO-TP receiver (`--kernel`). They need

```sh
sudo modprobe vcan; sudo modprobe can-isotp
sudo ip link add vcan0 type vcan && sudo ip link set vcan0 mtu 72 up
```

`CANBUS_REQUIRE_VCAN=1` turns a missing interface into a failure;
`CANBUS_REQUIRE_ISOTP=1` does the same for the can-isotp module, which
GitHub's hosted runners lack (the kernel ISO-TP test skips there and runs in
local Linux VMs).

**Results at the time of v0.1.0** (Linux arm64 VM, kernel 7.0, vcan0 and
can-isotp): all tests pass with `-race`; whole-module coverage 94.5 %
(macOS, without the vcan tests: 94.4 %).

**Not tested here:** real CAN controllers (bit timing changes, bus-off, error
counters) — `configure` with real hardware is covered by the request encoding
tests in go-canbus only.

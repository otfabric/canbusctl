# canbusctl

[![CI](https://github.com/otfabric/canbusctl/actions/workflows/ci.yml/badge.svg)](https://github.com/otfabric/canbusctl/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/otfabric/canbusctl/branch/main/graph/badge.svg)](https://codecov.io/gh/otfabric/canbusctl)

A command-line toolkit for CAN, CAN FD, ISO-TP and CAN databases (DBC, KCD,
ARXML): capture, decode, convert, diagnose, transmit, replay, simulate and
test under faults. Built on
[go-canbus](https://github.com/otfabric/go-canbus); pure Go, one static binary.

```sh
canbusctl monitor -i can0 --dbc vehicle.dbc                 # watch and decode, passively
canbusctl dbc encode vehicle.dbc Engine Speed=1500 Gear=Second
canbusctl send -i vcan0 123#DEADBEEF                         # vcan: no confirmation needed
canbusctl record -i can0 -o trip.blf --duration 10m          # .log .jsonl .pcap .asc .blf
canbusctl trace convert trip.blf trip.asc                    # reports what ASC cannot keep
canbusctl replay trip.blf -i vcan0 --speed 2
canbusctl dbc convert system.arxml --to kcd -o system.kcd
canbusctl bridge -a vcan0 -b vcan1 --drop 0.01 --delay-probability 0.1 --duration 1m
canbusctl isotp send -i vcan0 --tx-id 7E0 --rx-id 7E8 --data 0322F190
canbusctl simulate --dbc vehicle.dbc --duration 5s           # in-process virtual bus
canbusctl dbc generate vehicle.dbc -o vehicle/vehicle.go
```

## Commands

| Group | Commands | Transmits? |
|-------|----------|-----------|
| Interfaces | `interfaces`, `info`, `configure`, `up`, `down` | administrative changes need `--yes` |
| Frames | `monitor` (`-o` also records), `record`, `stats` | never |
| | `send`, `send-fd`, `replay` | yes |
| Databases | `dbc inspect`, `validate`, `decode`, `encode`, `generate`, `convert` (DBC, KCD, ARXML → DBC, KCD, JSON) | never (no interface) |
| Traces | `trace convert`, `trace info` (candump, JSON Lines, pcap/pcapng, ASC, BLF) | never (no interface) |
| ISO-TP | `isotp monitor` | never |
| | `isotp receive` | flow control frames only |
| | `isotp send` | yes |
| Simulation | `simulate`, `inject` | yes, unless `--interface virtual` |
| Fault testing | `bridge` (seeded drop, duplicate, delay, reorder, corrupt between two interfaces) | yes |
| Other | `version`, `completion` | |

Full reference: [docs/cli.md](docs/cli.md). Output: `--format text` (default),
`table`, or versioned `json` ([docs/json-output.md](docs/json-output.md)).
Exit codes: [docs/exitcodes.md](docs/exitcodes.md).

## Safety

CAN frames can move brakes, drives and power electronics. canbusctl is
**passive by default** and conservative when it is not:

- `monitor`, `record`, `stats`, `isotp monitor` and all `dbc` commands never
  transmit.
- Every transmitting command refuses to send on an interface that is not
  virtual — not `vcan`, `vxcan` or the in-process bus `virtual` — unless
  `--yes` is given (exit 9). The kind is read from the kernel (rtnetlink).
- Transmissions are bounded: `--max-frames` (default 10000), `--duration`,
  `--rate`; `simulate` always has a duration (default 10 s).
- `up`, `down` and `configure` need `--yes` and CAP_NET_ADMIN.
- Nothing probes the bus: canbusctl never guesses identifiers or sends traffic
  you did not ask for. `isotp receive` sends flow control frames, as any
  ISO-TP receiver must; `isotp monitor` does not.

## Platforms

| | Linux amd64, arm64, armv7 | macOS amd64, arm64 | Windows amd64, arm64 |
|-|:-:|:-:|:-:|
| SocketCAN interfaces (`interfaces`, `monitor -i can0`, `send`, ...) | ✓ | exit 8 | exit 8 |
| Kernel ISO-TP (`--kernel`) | ✓ (can-isotp module) | exit 8 | exit 8 |
| Userspace ISO-TP | ✓ | on `virtual` | on `virtual` |
| `dbc` commands, traces (candump, JSON Lines, pcap, pcapng, ASC, BLF) | ✓ | ✓ | ✓ |
| In-process virtual bus (`-i virtual`: simulate, inject, replay) | ✓ | ✓ | ✓ |

There is no hardware adapter support beyond SocketCAN (no PEAK, Kvaser, Vector
or slcan drivers on macOS/Windows): see [LIMITATIONS.md](LIMITATIONS.md).

## Install

Binaries for every platform above are attached to each
[GitHub release](https://github.com/otfabric/canbusctl/releases). From source:

```sh
go install github.com/otfabric/canbusctl@latest
```

Hardware-free experiments on Linux:

```sh
sudo ip link add vcan0 type vcan && sudo ip link set vcan0 mtu 72 up
canbusctl monitor -i vcan0 &
canbusctl send-fd -i vcan0 123##1DEADBEEF
```

ASC and BLF files hold local wall-clock times without a time zone: canbusctl
reads and writes them in the local zone. Set `TZ` (e.g. `TZ=UTC`) to match a
file recorded elsewhere.

## Documentation

[ARCHITECTURE.md](ARCHITECTURE.md) · [TESTING.md](TESTING.md) ·
[SECURITY.md](SECURITY.md) · [LIMITATIONS.md](LIMITATIONS.md) ·
[CHANGELOG.md](CHANGELOG.md) · [CONTRIBUTING.md](CONTRIBUTING.md) ·
[RELEASE.md](RELEASE.md)

## License

MIT, see [LICENSE](LICENSE).

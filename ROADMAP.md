# Gaps and follow-ups

What canbusctl does not do yet and the follow-ups worth doing, as of
v0.6.0 (on go-canbus v0.7.0, 2026-10-29). Work paused here; nothing below
is in progress. Library gaps (vendor adapters, higher-layer protocols,
hardware validation, database constructs) are in
[go-canbus ROADMAP.md](https://github.com/otfabric/go-canbus/blob/main/ROADMAP.md);
canbusctl inherits them. Details: [LIMITATIONS.md](LIMITATIONS.md). Effort:
**S** about a day, **M** a few days, **L** a week or more.

## Gaps

| # | Gap | Follow-up | Effort |
|---|-----|-----------|--------|
| 1 | CAN XL is not carried by `monitor`, `stats`, `bridge`, `inject` and `simulate` (only by the `xl` commands, `record --xl`, `replay --xl` and the trace commands) | An `--xl` flag on each: go-canbus's Bus, virtual bus and chaos bridge already carry CAN XL; `stats` would count CAN XL priorities apart | M |
| 2 | `xl monitor` and `record --xl` cannot filter on VCID (`--filter` selects by priority, as the kernel does) | A `--vcid <id>[:<mask>]` flag using `socketcan.WithVCIDFilter` / `virtual.WithVCIDFilter` (Linux ≥ 6.9) | S |
| 3 | No vendor adapters (PEAK, Kvaser, Vector): on Windows and macOS only slcan adapters and the virtual bus | Follows go-canbus transports; an `-i pcan:<channel>` style prefix like `slcan:` | with the library |
| 4 | No UDS, J1939 or CANopen commands (`isotp` moves bytes only) | `uds` (read DID, read/clear DTC, session control) once go-canbus has a UDS module; `j1939 monitor` with PGN decoding | L |
| 5 | `isotp monitor` follows one connection (two identifiers) | Follow several pairs, or every pair seen, with `--pairs` | S |
| 6 | No MF4 (ASAM MDF) traces | Follows go-canbus | with the library |
| 7 | No ARXML writer (`dbc convert` writes DBC, KCD and JSON) | Follows go-canbus | with the library |
| 8 | `simulate` generates values from patterns; it does not model ECU behaviour or answer requests | A scripted responder (request ID → response) for ISO-TP/UDS tests | M |
| 9 | Fault injection is userspace: no bit errors, CRC failures, arbitration loss, bus-off | Needs hardware (go-canbus ROADMAP V1) | — |

## Verification gaps

- No test against real CAN hardware; vcan and the virtual bus only (go-canbus
  ROADMAP V1).
- The interop suite exercises canbusctl against the canbus-interop images
  (python-can, socketcan-rs, can-utils, Scapy, cantools, canmatrix); CAN XL
  only against can-utils, the one stack with CAN XL.

## Suggested order

1. CAN XL in `monitor`, `stats`, `bridge`, `inject`, `simulate` (gap 1) and
   the VCID filter (gap 2): small, completes CAN XL in the tool.
2. `isotp monitor` with several pairs (gap 5).
3. UDS and J1939 commands once go-canbus has the modules (gap 4).

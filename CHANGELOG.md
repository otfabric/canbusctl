# Changelog

All notable changes are listed here; release notes are in
[RELEASE.md](RELEASE.md).

## v0.2.2 (2026-10-28)

Built on go-canbus v0.2.2: `monitor`, `record`, `stats` and the ISO-TP
commands no longer lose frames that arrive between opening the interface
and subscribing to it.

## v0.2.1 (2026-10-28)

Built on go-canbus v0.2.1, which reports a socket deadline that fires a
moment before the context's own as the context's deadline: canbusctl's own
workarounds for that edge case are removed (no behaviour change).

## v0.2.0 (2026-10-28)

Trace formats ASC and BLF everywhere (`record`, `replay`, `monitor -o`,
`dbc decode --trace`, `inject --trace`) and the `trace` group (`convert` with
loss reports, `info`); ARXML databases everywhere and `dbc convert --to kcd`;
`bridge`, a seeded fault bridge between two interfaces; `inject` runs on the
same chaos bridge (adds `--reorder`; its JSON `faults` gains `received`,
`forwarded` and `reordered`); `isotp receive/send --fc-delay` (N_Br). A
socket deadline that fires a moment before the command's own deadline now
ends `--timeout`/`--duration` runs normally (it was reported as a timeout or
I/O error). End-to-end, chaos and interop test suites; Kernels (microVM) and
Interop workflows. Built on go-canbus v0.2.0.

## v0.1.0 (2026-10-27)

First version: interface administration, monitor/record/replay/stats,
send/send-fd, the dbc command group, ISO-TP send/receive/monitor (userspace and
kernel), simulate and inject; text, table and JSON output; stable exit codes;
transmit safety checks. Built on go-canbus v0.1.0.

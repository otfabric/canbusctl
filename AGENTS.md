# AGENTS.md — canbusctl

## What this is

Command-line toolkit for CAN, CAN FD, ISO-TP and CAN databases (DBC, KCD, ARXML) built on [go-canbus](https://github.com/otfabric/go-canbus). Pure Go, one static binary. Profile: **cli**.

## Organisation standards

This repository follows **otfabric/engineering policy 1.0**, profile **cli** (`.otfabric/policy.yaml`, mode `baseline`). Where this file and the organisation standards disagree, this file wins here — say so in your report.

## Where the truth lives

| Question | Source |
|---|---|
| Exit codes (script-facing API) | docs/exitcodes.md |
| JSON output contract | docs/json-output.md, schemas/ |
| Command reference | docs/cli.md |
| What is tested and how | TESTING.md |
| Known limits | LIMITATIONS.md |
| Layout | ARCHITECTURE.md |
| CAN behaviour | go-canbus |
| Release history | RELEASE.md (`## vX.Y.Z` … `---`) |

## Hard rules

1. Exit codes and JSON output are a public contract: changing a code or a field is a breaking change with a RELEASE.md note.
2. A transmission on an interface that is not virtual and every administrative change require `--yes` (exit 9 otherwise). Read-only is the default.
3. Never lower `COVERAGE_MIN`, add lint suppressions, or skip or delete tests to make CI pass.
4. go-canbus is pinned to a released tag; no `replace` directives.

## Commands

| When you change… | Run |
|---|---|
| anything | `make check` |
| SocketCAN paths (Linux) | `make test-vcan` |
| input parsing | `make fuzz` |
| interop-relevant behaviour | `make interop` |

## Definition of Done

`make check` green; tests for new commands; docs/cli.md, exit codes and JSON documented; RELEASE.md section for user-visible changes.

## Git and releases

Branches and PRs. Do not push to `main`, tag or run the Release workflow unless the maintainer asks in the session.

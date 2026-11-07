# Security

## Reporting

Report vulnerabilities privately, not in public issues: email
**security@otfabric.io**, or open a private advisory on GitHub (the
[Security](https://github.com/otfabric/canbusctl/security) tab, "Report a
vulnerability"). Include the `canbusctl version` output.

## Supported versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | Best effort |

## Operational safety

canbusctl can transmit on CAN buses that control physical processes. It is
passive by default, requires `--yes` before transmitting on any interface
that is not virtual and before administrative changes, and bounds every
transmission (`--max-frames`, `--duration`, `--rate`). It does not probe
buses or guess identifiers. Use a test bus or a vcan interface first.

Raw CAN sockets need no privilege on Linux: any local user can monitor and
transmit on CAN interfaces. Restrict access to the host accordingly; the
`--yes` confirmation guards against mistakes, not against malicious users.

## Untrusted input

Databases, traces and frame notation may come from untrusted sources: their
parsers live in go-canbus and are fuzzed there; canbusctl's own parsers of
frame notation, `signal=value` and filters are fuzzed in this repository.
Generated Go code (`dbc generate`) embeds names from the database only as
sanitized identifiers and quoted strings.

# Security

## Reporting

Report vulnerabilities privately through GitHub's security advisories of
otfabric/canbusctl ("Report a vulnerability"), not in public issues.

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

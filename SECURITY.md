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
transmitting command (`--max-frames`, `--duration`, `--rate`; see Limits).
The exception is `isotp receive`: as any ISO-TP receiver it answers the
first frames and blocks it receives with flow control frames on `--tx-id`,
on any interface and without `--yes`; use `isotp monitor` to observe a
transfer without sending. It does not probe buses or guess identifiers. Use
a test bus or a vcan interface first.

Raw CAN sockets need no privilege on Linux: any local user can monitor and
transmit on CAN interfaces. Restrict access to the host accordingly; the
`--yes` confirmation guards against mistakes, not against malicious users.

## Untrusted input

Databases, traces and frame notation may come from untrusted sources: their
parsers live in go-canbus and are fuzzed there; canbusctl's own parsers of
frame notation, `signal=value` and filters are fuzzed in this repository.
Generated Go code (`dbc generate`) embeds names from the database only as
sanitized identifiers and quoted strings.

## Limits

The bounds that apply to untrusted input and to transmissions. A value
beyond a bound is refused with exit 3 unless the table says otherwise.

| What | Bound |
|------|-------|
| DBC and KCD database | 64 MiB (go-canbus); larger: exit 3 `invalid_database`. The file is streamed to the parser, so a larger file, a FIFO or a device is refused once the bound is read, not read whole (v0.13.0) |
| ARXML database | 1 GiB (go-canbus), streamed in the same way |
| Signals in a database | 262 144 (go-canbus); beyond them the rest of the file is not read (exit 3) |
| White space before the content of a database without a `.dbc`, `.kcd` or `.arxml` extension | 1 GiB, read to tell the kind (v0.13.0) |
| Identifiers listed by `trace info` and `stats --duration` | 65 536; the frames of further identifiers are counted in `otherFrames` (v0.13.0) |
| Cycle time `simulate` takes from a database (`GenMsgCycleTime`, KCD `interval`) | 1 ms .. 1 hour; outside it the cycle time is ignored with a warning and the message is treated as one without a cycle time (v0.13.0) |
| `--max-frames` of `send`, `send-fd`, `xl send`, `replay`, `simulate`, `inject` | default 10 000 frames (0: no limit); `inject` counts the duplicates of its fault stage (v0.13.0) |
| `--duration` of `simulate` | default 10 s; 0 or negative is refused |
| `inject --malformed` | 0 .. 65 536 frames |
| `isotp send --count` | 1 .. 10 000 messages |
| `isotp --max-size` | default 4095 bytes; `isotp send --size` beyond it is refused before the payload is built (exit 6), and never more than 4 294 967 295 bytes (ISO-TP's 32-bit length) are built |
| Receive queues of `monitor`, `record`, `stats` and the virtual bus | 65 536 frames; frames beyond a full queue are counted as `dropped` |
| Int flags | the platform's `int`: a value beyond 32 bits is a usage error (exit 2) on 32-bit platforms (v0.13.0) |
| `configure --sample-point`, `--dsample-point` | above 0 and below 100; NaN and infinities are refused (v0.13.0) |

Text and table output write the characters of database and trace strings a
terminal acts on (C0 and C1 control characters, DEL, Unicode bidirectional
formatting characters, bytes that are not UTF-8) as escapes such as `\x1b`
(v0.13.0); JSON carries the strings, escaped by JSON. Diagnostics on stderr
are not escaped.

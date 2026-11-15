# Process exit codes

canbusctl's exit codes are stable for scripting. A fatal error is rendered once:
as one `error:` line on stderr, or, with `--format json`, as a single JSON error
document on stdout (see [json-output.md](json-output.md)). `--format` is honoured
also for usage errors.

The table follows the organisation table
(otfabric/engineering `standards/api-design.md#cli-exit-codes`). Older versions used 7 for
`permission`, 10 for an interrupted command and 9 for a safety refusal; see
RELEASE.md.

| Code | Kind | Meaning |
|------|------|---------|
| 0 | | Success |
| 1 | | Findings: a result was written and it reports problems — `dbc validate` found errors (or warnings with `--strict`), `dbc decode` met frames that do not decode (or of unknown messages with `--strict`). No error document follows |
| 2 | `usage` | Unknown command or flag, missing required flag or argument, a value a flag cannot hold (an int flag holds the platform's `int`: 32 bits on 32-bit platforms) |
| 3 | `invalid_input` | A value, file, frame, database, trace or combination of flags that does not make sense. Nothing was transmitted |
| 4 | `transport` | The interface does not exist, is down, or failed while in use |
| 5 | `timeout` | A deadline passed: an ISO-TP peer did not answer (N_Bs) or `isotp receive` got fewer messages than `--count` in `--timeout` |
| 6 | `protocol` | ISO-TP protocol failure: receiver overflow, wrong sequence number, too many WAIT flow controls, invalid flow control |
| 8 | `unsupported` | Not available here: SocketCAN and netlink on macOS/Windows, bit timing on vcan, CAN FD on a classical interface, error frames on SocketCAN, `dbc convert --to kcd` |
| 9 | `permission` | A privilege is missing: CAP_NET_ADMIN for `up`, `down`, `configure` |
| 10 | `output`, `internal` | The result could not be written, or something unexpected happened |
| 13 | `safety` | Refused for safety: a transmission on an interface that is not virtual, or an administrative change, without `--yes` |
| 130 | `interrupted` | SIGINT or SIGTERM stopped a command before it had a result (`code` `canceled`). Streams stopped that way end with 0 |

`kind` selects the exit code; `code` in the JSON error is a stable, finer
identifier:

| `code` | Typical kinds |
|--------|---------------|
| `invalid_input` | `usage`, `invalid_input` |
| `invalid_frame` | `invalid_input` |
| `invalid_database` | `invalid_input` |
| `invalid_capture` | `invalid_input`; `internal` for `replay` from standard input after frames were sent |
| `unrepresentable_frame` | `invalid_input`: `trace convert` met a frame the target format cannot hold (such as a pcap before 1970) |
| `file_open_failed` | `invalid_input` |
| `interface_unavailable` | `transport` |
| `io_failed` | `transport` |
| `timeout` | `timeout` |
| `isotp_failed` | `timeout`, `protocol` |
| `permission_denied` | `permission` |
| `unsupported` | `unsupported` |
| `confirmation_required` | `safety` |
| `output_failed` | `output` |
| `canceled` | `interrupted` |
| `internal_unexpected` | `internal` |

**Streams.** `monitor`, `record`, `stats --duration`, `isotp receive -n 0`,
`isotp monitor`, `simulate` and `replay --print` end normally (exit 0) at
`--count`, `--duration` or Ctrl-C (SIGINT/SIGTERM).

**Interrupted transmissions.** `send`, `send-fd`, `xl send`, `replay` and
`inject` stopped by Ctrl-C (SIGINT/SIGTERM) also end with 0 and their
partial result: `sent` (`send`, `send-fd`, `xl send`), `read` and `sent`
(`replay`), `delivered` (`inject`). Compare it with what was asked for to
tell an interrupted run from a complete one. `isotp send` and `isotp receive
-n N` (N > 0), which have no partial result, end with 130.

**Transmitting commands** validate everything before the first frame: an
invocation that exits 2, 3, 8 or 13 has sent nothing. `replay` reads a whole
trace file before sending, so a malformed record anywhere in it exits 3.
From standard input (`-`) it cannot: a malformed record after frames were
sent exits 10 with code `invalid_capture` and the number of frames sent.

**Output files** (`record -o`, `monitor -o`, `xl monitor -o`, `trace
convert`, `dbc generate -o`, `dbc convert -o`) are created owner-only
(0600); an existing file keeps its mode. An output path that is a symbolic
link is refused with exit 3 (`file_open_failed`): name the target itself.
`trace convert` removes its partial output when it fails. `monitor -o`,
`record` and `xl monitor -o` open the interface before they create the
output, so an existing file is left alone when the interface does not open
(exit 4 or 8). A trace output that cannot be written or closed (a full
disk: the trace writers buffer, so the error may come at the end) exits 10
(`output_failed`).

# Process exit codes

canbusctl's exit codes are stable for scripting. A fatal error is rendered once:
as one `error:` line on stderr, or, with `--format json`, as a single JSON error
document on stdout (see [json-output.md](json-output.md)). `--format` is honoured
also for usage errors.

The table follows the organisation table
(otfabric/engineering `standards/api-design.md#cli-exit-codes`). Older versions used 7 for
`permission` and 10 for an interrupted command; see RELEASE.md.

| Code | Kind | Meaning |
|------|------|---------|
| 0 | | Success |
| 1 | | Findings: a result was written and it reports problems — `dbc validate` found errors (or warnings with `--strict`), `dbc decode` met frames that do not decode (or of unknown messages with `--strict`). No error document follows |
| 2 | `usage` | Unknown command or flag, missing required flag or argument, a value a flag cannot hold |
| 3 | `invalid_input` | A value, file, frame, database, trace or combination of flags that does not make sense. Nothing was transmitted |
| 4 | `transport` | The interface does not exist, is down, or failed while in use |
| 5 | `timeout` | A deadline passed: an ISO-TP peer did not answer (N_Bs) or `isotp receive` got fewer messages than `--count` in `--timeout` |
| 6 | `protocol` | ISO-TP protocol failure: receiver overflow, wrong sequence number, too many WAIT flow controls, invalid flow control |
| 8 | `unsupported` | Not available here: SocketCAN and netlink on macOS/Windows, bit timing on vcan, CAN FD on a classical interface, error frames on SocketCAN, `dbc convert --to kcd` |
| 9 | `safety`, `permission` | Refused for safety: a transmission on an interface that is not virtual, or an administrative change, without `--yes`. `permission`: a privilege is missing: CAP_NET_ADMIN for `up`, `down`, `configure` |
| 10 | `output`, `internal` | The result could not be written, or something unexpected happened |
| 130 | `interrupted` | SIGINT or SIGTERM stopped a command before it had a result (`code` `canceled`). Streams stopped that way end with 0 |

`kind` selects the exit code; `code` in the JSON error is a stable, finer
identifier:

| `code` | Typical kinds |
|--------|---------------|
| `invalid_input` | `usage`, `invalid_input` |
| `invalid_frame` | `invalid_input` |
| `invalid_database` | `invalid_input` |
| `invalid_capture` | `invalid_input` |
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

**Transmitting commands** validate everything before the first frame: an
invocation that exits 2, 3, 8 or 9 has sent nothing.

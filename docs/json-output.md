# JSON output

`--format json` (or `CANBUSCTL_FORMAT=json`; `CANBUSCTL_OUTPUT_FORMAT` is accepted too, `_FORMAT` wins) makes every command machine
readable. Every document carries `"schemaVersion": "1.0"`; the major version
changes only with an incompatible change. Schemas:
[schemas/result-v1.schema.json](../schemas/result-v1.schema.json),
[schemas/record-v1.schema.json](../schemas/record-v1.schema.json),
[schemas/error-v1.schema.json](../schemas/error-v1.schema.json).

## Results

Commands with one result (interfaces, info, configure, up, down, send, send-fd, xl send,
record, replay, stats, dbc inspect/validate/encode, isotp send, trace
convert/info, simulate, inject, bridge) write one indented document:

```json
{
  "schemaVersion": "1.0",
  "command": "dbc encode",
  "result": {
    "message": "Engine",
    "frame": {"id": 256, "idHex": "100", "extended": false, "fd": false, "brs": false,
              "esi": false, "remote": false, "error": false, "len": 8, "data": "3412142000000000"},
    "notation": "100#3412142000000000"
  }
}
```

`trace convert` reports `read`, `written`, `filtered` and `losses` (what the
target format could not keep, CAN XL frames in ASC/BLF/TRC included);
`trace info` the frame kinds (`classical`, `fd`, `xl`, `remote`, `errors`)
and per-identifier `ids` (`frames`, `rateHz`, `minLen`, `maxLen`; CAN XL
frames as `"XL <vcid><priority>"`, e.g. `"XL 05242"`); `replay` also
`skippedXL` (CAN XL frames not sent without `--xl`); `bridge` the counters of each
direction (`aToB`, `bToA`: `received`, `forwarded`, `dropped`, `duplicated`,
`delayed`, `reordered`, `corrupted`). `inject`'s `faults` has the same
counters (v0.2.0: `received`, `forwarded` and `reordered` are new).

## version

`version --json` (or `version` with `--format json`) is the exception: one
line, no envelope, the same keys in this order in every OT Fabric CLI:

```json
{"name":"canbusctl","version":"0.10.0","tag":"v0.10.0","commit":"1802d0f9afe8642d1e278120b76fcdfac967cb9e","built":"2026-11-05T15:41:49Z","libraries":[{"name":"go-canbus","version":"0.8.0"},{"name":"go-serial","version":"0.2.1"}],"go":"go1.26.10","platform":"linux/amd64"}
```

Every key is always present. A development build has `"version":"dev"`,
`"tag":""`, its VCS commit (`-dirty` with local changes) or `"unknown"`, and
`"built":"unknown"`. `libraries` lists the OT Fabric libraries
(`github.com/otfabric/go-*`) the binary is built on, sorted by name, with
their versions without the leading `v`; `[]` when there are none.

## Streams

Streaming commands (monitor, dbc decode, isotp receive/monitor, simulate and
replay with `--print`, inject) write JSON Lines: one compact object per line
with `schemaVersion` and `type`:

| `type` | Fields |
|--------|--------|
| `frame` | the frame fields above, plus `timestamp` (seconds with microseconds, as a string), `interface`, `echo`; decoded frames add `message` and `signals` (`name`, `raw` as an exact decimal string, `physical`, `unit`, `label`) or `decodeError`; error frames add `errorInfo` |
| `message` | ISO-TP: `len`, `data` (hex), `txId` (isotp monitor) |
| `note` | isotp monitor: `txId`, `note` |
| `frame` with `"kind": "xl"` | CAN XL frames of xl monitor and replay `--xl --print`: `priority`, `priorityHex`, `vcid`, `sec`, `rrs`, `sdt`, `af`, `len` (1..2048), `data`, `timestamp`, `interface`, `echo` (its CAN and CAN FD frames are ordinary `frame` records) |
| `summary` | the end of monitor (`frames`, `dropped`), xl monitor (also `xl`: the CAN XL frames), simulate, replay `--print` |

```json
{"schemaVersion":"1.0","type":"frame","timestamp":"1700000000.000000","interface":"vcan0","id":256,"idHex":"100","extended":false,"fd":false,"brs":false,"esi":false,"remote":false,"error":false,"len":8,"data":"3412142000000000","message":"Engine","signals":[{"name":"Speed","raw":"4660","physical":582.5,"unit":"rpm"}]}
```

## Errors

A fatal error is one document on stdout, and the exit code tells its kind
([exitcodes.md](exitcodes.md)):

```json
{
  "schemaVersion": "1.0",
  "error": {"kind": "safety", "code": "confirmation_required", "message": "send would transmit on can0, ..."}
}
```

Do not parse `message`; use `kind` and `code`.

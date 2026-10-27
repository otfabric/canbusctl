# JSON output

`--format json` (or `CANBUSCTL_FORMAT=json`) makes every command machine
readable. Every document carries `"schemaVersion": "1.0"`; the major version
changes only with an incompatible change. Schemas:
[schemas/result-v1.schema.json](../schemas/result-v1.schema.json),
[schemas/record-v1.schema.json](../schemas/record-v1.schema.json),
[schemas/error-v1.schema.json](../schemas/error-v1.schema.json).

## Results

Commands with one result (interfaces, info, configure, up, down, send, send-fd,
record, replay, stats, dbc inspect/validate/encode, isotp send, simulate,
inject, version) write one indented document:

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

## Streams

Streaming commands (monitor, dbc decode, isotp receive/monitor, simulate and
replay with `--print`, inject) write JSON Lines: one compact object per line
with `schemaVersion` and `type`:

| `type` | Fields |
|--------|--------|
| `frame` | the frame fields above, plus `timestamp` (seconds with microseconds, as a string), `interface`, `echo`; decoded frames add `message` and `signals` (`name`, `raw` as an exact decimal string, `physical`, `unit`, `label`) or `decodeError`; error frames add `errorInfo` |
| `message` | ISO-TP: `len`, `data` (hex), `txId` (isotp monitor) |
| `note` | isotp monitor: `txId`, `note` |
| `summary` | the end of monitor (`frames`, `dropped`), simulate, replay `--print` |

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

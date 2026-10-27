# Architecture

canbusctl is a thin command layer over go-canbus. It owns parsing, output,
exit codes and safety; every CAN operation is a library call.

```
main.go                 build metadata (ldflags), cmd.Execute
cmd/                    one file per command group (cobra)
  root.go               Run(ctx, args, stdin, stdout, stderr) → exit code; --format; version; completion
  interfaces.go         interfaces, info, configure, up, down     → go-canbus/socketcan/link
  frames.go             monitor, send, send-fd, record, replay, stats → canbus.Bus, capture
  dbc.go                dbc inspect/validate/decode/encode/generate/convert → candb, dbc, kcd, codegen
  isotp.go              isotp send/receive (isotp, socketcan.DialISOTP), monitor (listen-only isotp.Conn)
  simulate.go           deterministic traffic from a database
  inject.go             fault stage (virtual.Bus with seeded faults) in front of the target
  transport.go          opens an interface: socketcan.Open, or a port of the in-process virtual bus
internal/cli            identifiers, frames, filters, signal=value, databases, traces, transmit safety
internal/output         text, table, JSON envelope, JSON Lines streams
internal/errs           kinds, stable codes, exit codes, classification of go-canbus errors
```

**Run, not os.Exit.** `cmd.Run` takes its streams and returns the exit code,
so tests run the real command tree in process (`cmd/*_test.go`), including
usage errors.

**The in-process virtual bus.** `--interface virtual` gives every command a
port of one go-canbus `virtual.Bus` per invocation: `simulate`, `inject` and
`replay --print` work on every OS without interfaces; tests drive commands by
sending on the same bus.

**Safety** lives in one function, `cli.TransmitCheck`: the in-process bus,
vcan and vxcan (kind from rtnetlink) pass; anything else needs `--yes`.
Administrative commands need `--yes` regardless.

**Errors.** Commands return typed `errs.Error`s or go-canbus errors;
`errs.Classify` maps go-canbus sentinels (ErrUnsupported, ErrInterfaceDown,
isotp.ErrTimeout, candb.ErrRange, ...) to kinds and exit codes. cobra's own
command-line errors are usage errors (2).

**Listen-only ISO-TP.** `isotp monitor` opens two go-canbus ISO-TP
connections with `ListenOnly` (one per direction): they reassemble messages
and never transmit.

**Human numbers.** Text and table output print physical values with at most
12 significant digits (`human` in `cmd/frames.go`); JSON keeps the full
float64 and the raw value as an exact decimal string. `dbc encode` passes
`signal=value` numbers to go-canbus as decimal text (`candb.PhysText`), so a
64-bit raw value of a factor-1 signal is encoded exactly.

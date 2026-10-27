# Contributing

1. Work on a branch; open a pull request against `main`.
2. Run `make check` (and `make test-vcan` on Linux with vcan0) before pushing.
3. New commands follow the existing shape: a `newXxxCmd(a *app)` constructor,
   validation before any transmission, typed errors from `internal/errs`, a
   result type with `Text()` (and `Headers()`/`Rows()` for tables), tests
   through `cmd.Run`.
4. Anything that transmits goes through `cli.TransmitCheck` and has
   `--max-frames`/`--duration`/`--rate` bounds.
5. CAN logic (transports, codecs, ISO-TP, capture formats) belongs in
   go-canbus, not here.
6. Regenerate the reference after changing help texts: `scripts/gen-cli-doc.sh`.
7. Add an SPDX header (`// SPDX-License-Identifier: MIT`) to new Go files.
8. Commit messages: imperative subject, a body that says why.

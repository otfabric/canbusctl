// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/codegen"
	"github.com/otfabric/go-canbus/dbc"
	"github.com/otfabric/go-canbus/kcd"
	"github.com/spf13/cobra"
)

func newDBCCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "dbc",
		Short: "Inspect, validate, decode, encode, generate and convert CAN databases (DBC, KCD, ARXML)",
		Long: `The dbc commands take DBC files, KCD (Kayak) files and AUTOSAR 4.x ARXML
system descriptions (their CAN part), recognised by the .kcd and .arxml
extensions or by content. DBC is a database format describing messages and
signals, not a protocol. None of these commands touches a CAN interface.`,
	}
	c.AddCommand(newDBCInspectCmd(a), newDBCValidateCmd(a), newDBCDecodeCmd(a), newDBCEncodeCmd(a), newDBCGenerateCmd(a), newDBCConvertCmd(a))
	return c
}

type messageSummary struct {
	Name      string   `json:"name"`
	ID        uint32   `json:"id"`
	IDHex     string   `json:"idHex"`
	Extended  bool     `json:"extended"`
	Length    int      `json:"length"`
	FD        bool     `json:"fd"`
	CycleTime int      `json:"cycleTime,omitempty"`
	Senders   []string `json:"senders,omitempty"`
	Signals   int      `json:"signals"`
	Mux       bool     `json:"multiplexed"`
}

type inspectResult struct {
	Database    string             `json:"database"`
	Format      string             `json:"format"`
	Version     string             `json:"version,omitempty"`
	Nodes       []string           `json:"nodes"`
	Messages    []messageSummary   `json:"messages"`
	Message     *candb.Message     `json:"message,omitempty"`
	Diagnostics []candb.Diagnostic `json:"diagnostics"`
}

func idHex(id canbus.ID, ext bool) string {
	if ext {
		return fmt.Sprintf("%08X", uint32(id))
	}
	return fmt.Sprintf("%03X", uint32(id))
}

func (r inspectResult) Text() string {
	var b strings.Builder
	if m := r.Message; m != nil {
		fmt.Fprintf(&b, "%s  id %s  %d bytes", m.Name, idHex(m.ID, m.Extended), m.Length)
		if m.FD {
			b.WriteString("  CAN FD")
		}
		if m.CycleTime > 0 {
			fmt.Fprintf(&b, "  every %d ms", m.CycleTime)
		}
		b.WriteString("\n")
		if m.Comment != "" {
			fmt.Fprintf(&b, "  %s\n", m.Comment)
		}
		for _, s := range m.Signals {
			sign := "unsigned"
			if s.Signed {
				sign = "signed"
			}
			fmt.Fprintf(&b, "  %-20s bit %3d len %2d %-13s %-8s ×%s %s %s", s.Name, s.StartBit, s.Length, s.ByteOrder, sign, human(s.Factor), signed(s.Offset), s.Unit)
			if s.IsMultiplexer {
				b.WriteString(" [multiplexer]")
			}
			if s.Multiplexer != "" {
				fmt.Fprintf(&b, " [when %s in %v]", s.Multiplexer, s.MuxValues)
			}
			b.WriteString("\n")
		}
		return b.String()
	}
	fmt.Fprintf(&b, "%s (%s): %d messages, %d nodes\n", r.Database, r.Format, len(r.Messages), len(r.Nodes))
	for _, m := range r.Messages {
		fmt.Fprintf(&b, "  %-9s %-28s %2d bytes %3d signals", m.IDHex, m.Name, m.Length, m.Signals)
		if m.FD {
			b.WriteString(" FD")
		}
		if m.Mux {
			b.WriteString(" mux")
		}
		if m.CycleTime > 0 {
			fmt.Fprintf(&b, " %d ms", m.CycleTime)
		}
		b.WriteString("\n")
	}
	for _, d := range r.Diagnostics {
		b.WriteString("  " + d.String() + "\n")
	}
	return b.String()
}

func (r inspectResult) Headers() []string {
	return []string{"ID", "NAME", "LENGTH", "SIGNALS", "FD", "CYCLE_MS", "SENDERS"}
}

func (r inspectResult) Rows() [][]string {
	var rows [][]string
	for _, m := range r.Messages {
		rows = append(rows, []string{m.IDHex, m.Name, strconv.Itoa(m.Length), strconv.Itoa(m.Signals), strconv.FormatBool(m.FD), strconv.Itoa(m.CycleTime), strings.Join(m.Senders, ",")})
	}
	return rows
}

func newDBCInspectCmd(a *app) *cobra.Command {
	var message string
	c := &cobra.Command{
		Use:   "inspect <database>",
		Short: "List the messages of a database, or the signals of one (--message)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			db, diags, kind, err := cli.LoadDatabase(args[0])
			if err != nil {
				return err
			}
			r := inspectResult{Database: args[0], Format: kind, Version: db.Version, Nodes: []string{}, Messages: []messageSummary{}, Diagnostics: diags}
			if r.Diagnostics == nil {
				r.Diagnostics = []candb.Diagnostic{}
			}
			for _, n := range db.Nodes {
				r.Nodes = append(r.Nodes, n.Name)
			}
			for _, m := range db.Messages {
				r.Messages = append(r.Messages, messageSummary{Name: m.Name, ID: uint32(m.ID), IDHex: idHex(m.ID, m.Extended), Extended: m.Extended,
					Length: m.Length, FD: m.FD, CycleTime: m.CycleTime, Senders: m.Senders, Signals: len(m.Signals), Mux: m.Multiplexed()})
			}
			if message != "" {
				r.Message = findMessage(db, message)
				if r.Message == nil {
					return errs.Invalid("no message %q in %s", message, args[0])
				}
			}
			return a.emit("dbc inspect", r)
		},
	}
	c.Flags().StringVarP(&message, "message", "m", "", "Show the signals of this message (name or hexadecimal identifier)")
	return c
}

// signed formats an offset with its sign.
func signed(f float64) string {
	if f < 0 {
		return human(f)
	}
	return "+" + human(f)
}

// findMessage finds a message by name or hexadecimal identifier.
func findMessage(db *candb.Database, s string) *candb.Message {
	if m := db.MessageByName(s); m != nil {
		return m
	}
	if v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 16, 32); err == nil {
		if m := db.Message(canbus.ID(v), false); m != nil {
			return m
		}
		return db.Message(canbus.ID(v), true)
	}
	return nil
}

type validateResult struct {
	Database    string             `json:"database"`
	Valid       bool               `json:"valid"`
	Errors      int                `json:"errors"`
	Warnings    int                `json:"warnings"`
	Diagnostics []candb.Diagnostic `json:"diagnostics"`
}

func (r validateResult) Text() string {
	var b strings.Builder
	for _, d := range r.Diagnostics {
		b.WriteString(r.Database + ": " + d.String() + "\n")
	}
	status := "valid"
	if !r.Valid {
		status = "invalid"
	}
	fmt.Fprintf(&b, "%s: %s (%d errors, %d warnings)", r.Database, status, r.Errors, r.Warnings)
	return b.String()
}

func newDBCValidateCmd(a *app) *cobra.Command {
	var strict bool
	c := &cobra.Command{
		Use:   "validate <database>",
		Short: "Report every diagnostic of a database; exit 1 on errors (or warnings with --strict)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, diags, _, err := cli.LoadDatabase(args[0])
			var e *errs.Error
			if err != nil && (!errors.As(err, &e) || e.Code != errs.CodeInvalidDatabase) {
				return err
			}
			r := validateResult{Database: args[0], Diagnostics: diags}
			if r.Diagnostics == nil {
				r.Diagnostics = []candb.Diagnostic{}
			}
			for _, d := range diags {
				switch d.Severity {
				case candb.Error:
					r.Errors++
				case candb.Warning:
					r.Warnings++
				default:
				}
			}
			r.Valid = r.Errors == 0 && (!strict || r.Warnings == 0)
			if err := a.emit("dbc validate", r); err != nil {
				return err
			}
			if !r.Valid {
				return errs.ErrFindings
			}
			return nil
		},
	}
	c.Flags().BoolVar(&strict, "strict", false, "Treat warnings as errors")
	return c
}

func newDBCDecodeCmd(a *app) *cobra.Command {
	var (
		trace       string
		unknownFail bool
	)
	c := &cobra.Command{
		Use:   "decode <database> [frame...]",
		Short: "Decode frames given as arguments, in a trace file (--trace) or on stdin",
		Long: `decode prints the signals of each frame of a known message. Frames come as
cansend notation arguments, from a candump/JSON Lines/pcap/ASC/BLF trace (--trace, "-"
for stdin), or, without either, as lines of cansend notation or candump log on
stdin. A frame that does not decode (too short, bad multiplexer) makes the exit
code 1; frames of unknown messages do too with --strict.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			db, _, _, err := cli.LoadDatabase(args[0])
			if err != nil {
				return err
			}
			next, done, err := frameSource(a, args[1:], trace)
			if err != nil {
				return err
			}
			defer done()
			st := a.stream()
			failed := false
			for {
				f, err := next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return err
				}
				r, text := record(&f, db)
				if r.Decode != "" || unknownFail && r.Message == "" {
					failed = true
				}
				if r.Message == "" && !f.IsError() {
					text += "  (unknown message)"
				}
				if err := st.Record("frame", text, r); err != nil {
					return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
				}
			}
			if failed {
				return errs.ErrFindings
			}
			return nil
		},
	}
	c.Flags().StringVar(&trace, "trace", "", "Read frames from this trace file (\"-\": stdin)")
	c.Flags().BoolVar(&unknownFail, "strict", false, "Exit 1 also for frames of unknown messages")
	return c
}

// frameSource yields frames from arguments, a trace, or stdin lines.
func frameSource(a *app, args []string, trace string) (func() (canbus.Frame, error), func(), error) {
	if trace != "" {
		r, closer, err := cli.OpenTrace(trace, a.in)
		if err != nil {
			return nil, nil, err
		}
		return func() (canbus.Frame, error) {
			var f canbus.Frame
			err := r.Read(&f)
			if err != nil && !errors.Is(err, io.EOF) {
				err = &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture, Message: err.Error(), Cause: err}
			}
			return f, err
		}, closer, nil
	}
	lines := args
	if len(lines) == 0 {
		var err error
		if lines, err = cli.ReadLines(a.in); err != nil {
			return nil, nil, err
		}
	}
	i := 0
	return func() (canbus.Frame, error) {
		if i >= len(lines) {
			return canbus.Frame{}, io.EOF
		}
		l := lines[i]
		i++
		if strings.HasPrefix(l, "(") {
			var f canbus.Frame
			var err error
			if perr := captureLine(l, &f); perr != nil {
				err = errs.New(errs.KindInvalidInput, errs.CodeInvalidFrame, "line %d: %v", i, perr)
			}
			return f, err
		}
		var f canbus.Frame
		if err := captureFrame(l, &f); err != nil {
			return f, errs.New(errs.KindInvalidInput, errs.CodeInvalidFrame, "%v", err)
		}
		return f, nil
	}, func() {}, nil
}

type encodeResult struct {
	Message  string        `json:"message"`
	Frame    cli.FrameJSON `json:"frame"`
	Notation string        `json:"notation"`
}

func (r encodeResult) Text() string { return r.Notation }

func newDBCEncodeCmd(a *app) *cobra.Command {
	var raw, lax, fill bool
	c := &cobra.Command{
		Use:   "encode <database> <message> [signal=value...]",
		Short: "Encode a message from signal values into a frame (cansend notation)",
		Long: `Values are physical (scaled) numbers, value-table names (Gear=Second), or raw
integers with --raw. Every signal the payload carries must be given (multiplexer
values select which), unless --fill-missing uses initial values. Physical values
outside a signal's [min, max] are rejected unless --lax.`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			db, _, _, err := cli.LoadDatabase(args[0])
			if err != nil {
				return err
			}
			m := findMessage(db, args[1])
			if m == nil {
				return errs.Invalid("no message %q in %s", args[1], args[0])
			}
			in, err := cli.ParseAssignments(args[2:], raw)
			if err != nil {
				return err
			}
			f, err := m.Frame(in, candb.EncodeOptions{Lax: lax, FillMissing: fill})
			if err != nil {
				return err
			}
			return a.emit("dbc encode", encodeResult{Message: m.Name, Frame: cli.ToJSON(&f), Notation: f.String()})
		},
	}
	c.Flags().BoolVar(&raw, "raw", false, "Values are raw integers")
	c.Flags().BoolVar(&lax, "lax", false, "Do not check physical values against the signal range")
	c.Flags().BoolVar(&fill, "fill-missing", false, "Encode missing signals with their initial value (or 0)")
	return c
}

// writeOut writes to path, or to stdout for "" or "-".
func (a *app) writeOut(path string, data []byte) error {
	if path == "" || path == "-" {
		if _, err := a.out.Write(data); err != nil {
			return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", path, err)
	}
	return nil
}

func newDBCGenerateCmd(a *app) *cobra.Command {
	var pkg, out string
	var asJSON bool
	c := &cobra.Command{
		Use:   "generate <database>",
		Short: "Generate typed Go code (or a JSON descriptor) from a database",
		Long: `generate writes gofmt-formatted Go: one struct per message with typed raw fields,
identifier constants, enums, physical accessors and Encode/Decode/Frame methods.
The output is deterministic and names the source file and its SHA-256. It imports
github.com/otfabric/go-canbus. --json writes the format-independent JSON
descriptor instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			src, err := os.ReadFile(args[0])
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot read database: %w", err)
			}
			db, _, _, err := cli.LoadDatabase(args[0])
			if err != nil {
				return err
			}
			sum := sha256.Sum256(src)
			if pkg == "" {
				pkg = goPackageName(args[0])
			}
			opts := codegen.Options{Package: pkg, Source: filepath.Base(args[0]), SourceSHA256: hex.EncodeToString(sum[:])}
			var code []byte
			if asJSON {
				code, err = codegen.GenerateJSON(db, opts)
			} else {
				code, err = codegen.Generate(db, opts)
			}
			if err != nil {
				return errs.Invalid("%v", err)
			}
			return a.writeOut(out, code)
		},
	}
	c.Flags().StringVarP(&pkg, "package", "p", "", "Go package name (default: from the file name)")
	c.Flags().StringVarP(&out, "output", "o", "", "Output file (default: stdout)")
	c.Flags().BoolVar(&asJSON, "json", false, "Write the JSON descriptor instead of Go")
	return c
}

// goPackageName derives a package name from a file name.
func goPackageName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' && b.Len() > 0 {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "candb"
	}
	return b.String()
}

func newDBCConvertCmd(a *app) *cobra.Command {
	var to, out string
	c := &cobra.Command{
		Use:   "convert <database>",
		Short: "Convert a database: DBC, KCD or ARXML to DBC, KCD or the JSON model",
		Long: `Reads DBC, KCD or AUTOSAR 4.x ARXML (the CAN subset) and writes DBC, KCD
or the JSON model (--to dbc|kcd|json). Writing ARXML is not supported.
KCD cannot carry extended multiplexing (an error), attributes, value tables
and initial values (dropped, reported as warnings on stderr).`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			db, _, _, err := cli.LoadDatabase(args[0])
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			switch strings.ToLower(to) {
			case "dbc":
				if err := dbc.Write(&buf, db); err != nil {
					return err
				}
			case "kcd":
				diags, err := kcd.Write(&buf, db)
				for _, d := range diags {
					fmt.Fprintln(a.errOut, d.String())
				}
				if err != nil {
					return errs.New(errs.KindUnsupported, errs.CodeUnsupported, "%v", err)
				}
			case "json":
				js, err := codegen.GenerateJSON(db, codegen.Options{Source: filepath.Base(args[0])})
				if err != nil {
					return err
				}
				buf.Write(js)
			default:
				return errs.New(errs.KindUnsupported, errs.CodeUnsupported, "--to %q is not supported (dbc, kcd or json)", to)
			}
			return a.writeOut(out, buf.Bytes())
		},
	}
	c.Flags().StringVar(&to, "to", "", "Target format: dbc, kcd or json (required)")
	c.Flags().StringVarP(&out, "output", "o", "", "Output file (default: stdout)")
	_ = c.MarkFlagRequired("to")
	return c
}

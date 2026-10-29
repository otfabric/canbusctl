// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
	"github.com/spf13/cobra"
)

func newTraceCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "trace",
		Short: "Convert and inspect trace files (candump, JSON Lines, pcap/pcapng, ASC, BLF, TRC)",
		Long: `The trace commands work on files only and never touch an interface.

Readable: candump logs (.log/.txt), JSON Lines (.jsonl), pcap and pcapng,
Vector ASC (.asc) and BLF (.blf), PEAK TRC (.trc, versions 1.0 to 2.1;
written as 2.1), detected from the content. Writable: all of them, chosen
by the output's extension or --file-format. CAN XL frames are kept by
candump logs, JSON Lines, pcap and pcapng.

ASC, BLF and TRC hold local wall-clock times without a time zone: they are
read and written in the local zone; set TZ (e.g. TZ=UTC) to match a file
from another zone.`,
	}
	c.AddCommand(newTraceConvertCmd(a), newTraceInfoCmd(a))
	return c
}

type convertResult struct {
	Input    string   `json:"input"`
	Output   string   `json:"output"`
	Format   string   `json:"format"`
	Read     int      `json:"read"`
	Written  int      `json:"written"`
	Filtered int      `json:"filtered"`
	Losses   []string `json:"losses,omitempty"`
}

func (r convertResult) Text() string {
	s := fmt.Sprintf("%s → %s (%s): %d frames read, %d written", r.Input, r.Output, r.Format, r.Read, r.Written)
	if r.Filtered > 0 {
		s += fmt.Sprintf(", %d filtered out", r.Filtered)
	}
	for _, l := range r.Losses {
		s += "\n  lost: " + l
	}
	return s
}

// losses tracks what the target format cannot keep.
type losses struct {
	errorClass, iface, echo, subMicro, xl int
}

func (l *losses) note(f *canbus.Frame, format capture.Format) {
	switch format {
	case capture.FormatASC, capture.FormatBLF, capture.FormatTRC:
		if f.IsError() {
			// TRC keeps a protocol violation and the error counters.
			info, _ := canbus.ParseError(f)
			if format != capture.FormatTRC || info.Class&^(canbus.ErrClassProtocol|canbus.ErrClassCounters) != 0 {
				l.errorClass++
			}
		}
		if _, err := strconv.Atoi(f.Interface); f.Interface != "" && err != nil {
			l.iface++
		}
	case capture.FormatCandump:
		if f.Echo {
			l.echo++
		}
		if f.Timestamp.Nanosecond()%1000 != 0 {
			l.subMicro++
		}
	case capture.FormatPCAP:
		if f.Echo {
			l.echo++
		}
		if f.Interface != "" {
			l.iface++
		}
	case capture.FormatPCAPNG:
		if f.Echo {
			l.echo++
		}
	default:
	}
}

// noteXL records what the target format loses of a CAN XL frame. It
// reports false when the format cannot hold CAN XL at all.
func (l *losses) noteXL(x *canbus.XLFrame, format capture.Format) bool {
	f := canbus.Frame{Interface: x.Interface, Echo: x.Echo, Timestamp: x.Timestamp}
	switch format {
	case capture.FormatCandump, capture.FormatPCAP, capture.FormatPCAPNG:
		l.note(&f, format)
	case capture.FormatJSONL:
	default:
		l.xl++
		return false
	}
	return true
}

func (l *losses) list() []string {
	var out []string
	add := func(n int, what string) {
		if n > 0 {
			out = append(out, fmt.Sprintf("%d frames: %s", n, what))
		}
	}
	add(l.errorClass, "error class (ASC/BLF carry none, TRC only protocol violations and counters; read back as protocol errors)")
	add(l.iface, "interface name (ASC/BLF/TRC keep channel numbers, pcap none)")
	add(l.echo, "own-transmission (echo) mark")
	add(l.subMicro, "timestamp below microseconds")
	add(l.xl, "CAN XL, not written (ASC, BLF and TRC cannot carry CAN XL)")
	return out
}

func newTraceConvertCmd(a *app) *cobra.Command {
	var (
		ff        string
		filters   []string
		join      bool
		noErrors  bool
		maxFrames int
	)
	c := &cobra.Command{
		Use:   "convert <input> <output>",
		Short: "Convert a trace to another format, optionally filtered",
		Long: `convert reads <input> ("-": stdin) in any readable format and writes
<output> in the format of its extension (or --file-format). --filter keeps
only matching frames (candump notation, repeatable, ORed or ANDed with
--join); --no-errors drops error frames; --max-frames stops early. What the
target format cannot keep (error class in ASC/BLF, interface names, echo
marks, sub-microsecond times, CAN XL frames in ASC/BLF/TRC) is reported,
never silently dropped. --filter selects CAN XL frames by priority, as the
kernel's filters do.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			format, err := cli.TraceFormat(args[1], ff)
			if err != nil {
				return err
			}
			var fs *canbus.FilterSet
			if len(filters) > 0 {
				set, err := cli.ParseFilters(filters, join, false)
				if err != nil {
					return err
				}
				fs = &set
			}
			r, closer, err := cli.OpenTrace(args[0], a.in)
			if err != nil {
				return err
			}
			defer closer()
			file, err := os.Create(args[1])
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot create %s: %w", args[1], err)
			}
			defer func() { _ = file.Close() }()
			w, err := capture.NewWriter(file, format)
			if err != nil {
				return err
			}
			res := convertResult{Input: args[0], Output: args[1], Format: string(format)}
			var lost losses
			xr, _ := r.(capture.XLReader)
			xw, _ := w.(capture.XLWriter)
			var x canbus.XLFrame
			for maxFrames <= 0 || res.Written < maxFrames {
				var f canbus.Frame
				var isXL bool
				var err error
				if xr != nil {
					isXL, err = xr.ReadAny(&f, &x)
				} else {
					err = r.Read(&f)
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture, Message: fmt.Sprintf("%s: %v", args[0], err), Cause: err}
				}
				res.Read++
				if isXL {
					if fs != nil && !fs.MatchXL(&x) {
						res.Filtered++
						continue
					}
					if !lost.noteXL(&x, format) {
						continue
					}
					if err := xw.WriteXL(&x); err != nil {
						return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", args[1], err)
					}
					res.Written++
					continue
				}
				if f.IsError() && noErrors || !f.IsError() && fs != nil && !fs.Match(&f) {
					res.Filtered++
					continue
				}
				lost.note(&f, format)
				if err := w.Write(&f); err != nil {
					return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", args[1], err)
				}
				res.Written++
			}
			if err := w.Close(); err != nil {
				return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", args[1], err)
			}
			res.Losses = lost.list()
			return a.emit("trace convert", res)
		},
	}
	f := c.Flags()
	f.StringVar(&ff, "file-format", "", "candump, jsonl, pcap, pcapng, asc, blf or trc (default: from the extension)")
	f.StringArrayVar(&filters, "filter", nil, "Keep frames matching <id>:<mask> (repeatable)")
	f.BoolVar(&join, "join", false, "A frame must pass all filters")
	f.BoolVar(&noErrors, "no-errors", false, "Drop error frames")
	f.IntVar(&maxFrames, "max-frames", 0, "Stop after writing this many frames (0: all)")
	return c
}

type traceID struct {
	ID     string  `json:"id"`
	Frames int     `json:"frames"`
	RateHz float64 `json:"rateHz"`
	MinLen int     `json:"minLen"`
	MaxLen int     `json:"maxLen"`
}

type traceInfo struct {
	File      string    `json:"file"`
	Format    string    `json:"format"`
	Frames    int       `json:"frames"`
	First     string    `json:"first,omitempty"`
	Last      string    `json:"last,omitempty"`
	DurationS float64   `json:"durationSeconds"`
	Classical int       `json:"classical"`
	FD        int       `json:"fd"`
	Remote    int       `json:"remote"`
	XL        int       `json:"xl"`
	Errors    int       `json:"errors"`
	Extended  int       `json:"extended"`
	IDs       []traceID `json:"ids"`
}

func (r traceInfo) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): %d frames over %.6f s", r.File, r.Format, r.Frames, r.DurationS)
	if r.First != "" {
		fmt.Fprintf(&b, " (%s .. %s)", r.First, r.Last)
	}
	fmt.Fprintf(&b, "\n  classical %d, CAN FD %d, CAN XL %d, remote %d, error %d; extended identifiers %d\n", r.Classical, r.FD, r.XL, r.Remote, r.Errors, r.Extended)
	for _, id := range r.IDs {
		fmt.Fprintf(&b, "  %-9s %7d frames  %9.2f Hz  len %d..%d\n", id.ID, id.Frames, id.RateHz, id.MinLen, id.MaxLen)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r traceInfo) Headers() []string {
	return []string{"ID", "FRAMES", "RATE_HZ", "MIN_LEN", "MAX_LEN"}
}

func (r traceInfo) Rows() [][]string {
	var rows [][]string
	for _, id := range r.IDs {
		rows = append(rows, []string{id.ID, strconv.Itoa(id.Frames), strconv.FormatFloat(id.RateHz, 'f', 2, 64),
			strconv.Itoa(id.MinLen), strconv.Itoa(id.MaxLen)})
	}
	return rows
}

func frameKey(f *canbus.Frame) string {
	switch {
	case f.IsError():
		return "error"
	case f.IsExtended():
		return fmt.Sprintf("%08X", uint32(f.ID))
	}
	return fmt.Sprintf("%03X", uint32(f.ID))
}

func newTraceInfoCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "info <file>",
		Short: "Summarize a trace: frames, duration, kinds, per-identifier counts and rates",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := args[0]
			var src *os.File
			if path != "-" {
				f, err := os.Open(path)
				if err != nil {
					return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot open trace: %w", err)
				}
				defer func() { _ = f.Close() }()
				src = f
			}
			var r capture.Reader
			var format capture.Format
			var err error
			if src != nil {
				r, format, err = capture.NewReader(src)
			} else {
				r, format, err = capture.NewReader(a.in)
			}
			if err != nil {
				return &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture, Message: fmt.Sprintf("%s: %v", path, err), Cause: err}
			}
			res := traceInfo{File: path, Format: string(format)}
			ids := map[string]*traceID{}
			var first, last time.Time
			stamp := func(ts time.Time) {
				if !ts.IsZero() {
					if first.IsZero() || ts.Before(first) {
						first = ts
					}
					if ts.After(last) {
						last = ts
					}
				}
			}
			count := func(k string, n int) {
				s := ids[k]
				if s == nil {
					s = &traceID{ID: k, MinLen: n, MaxLen: n}
					ids[k] = s
				}
				s.Frames++
				s.MinLen, s.MaxLen = min(s.MinLen, n), max(s.MaxLen, n)
			}
			xr, _ := r.(capture.XLReader)
			var x canbus.XLFrame
			for {
				var f canbus.Frame
				var isXL bool
				var err error
				if xr != nil {
					isXL, err = xr.ReadAny(&f, &x)
				} else {
					err = r.Read(&f)
				}
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture, Message: fmt.Sprintf("%s: %v", path, err), Cause: err}
				}
				res.Frames++
				if isXL {
					res.XL++
					stamp(x.Timestamp)
					count(fmt.Sprintf("XL %02X%03X", x.VCID, x.Priority), len(x.Data))
					continue
				}
				switch {
				case f.IsError():
					res.Errors++
				case f.IsRemote():
					res.Remote++
				case f.IsFD():
					res.FD++
				default:
					res.Classical++
				}
				if f.IsExtended() {
					res.Extended++
				}
				stamp(f.Timestamp)
				count(frameKey(&f), int(f.Len))
			}
			if !first.IsZero() {
				res.First, res.Last = first.Format(time.RFC3339Nano), last.Format(time.RFC3339Nano)
				res.DurationS = last.Sub(first).Seconds()
			}
			for _, s := range ids {
				if res.DurationS > 0 {
					s.RateHz = float64(s.Frames) / res.DurationS
				}
				res.IDs = append(res.IDs, *s)
			}
			sort.Slice(res.IDs, func(i, j int) bool { return res.IDs[i].ID < res.IDs[j].ID })
			return a.emit("trace info", res)
		},
	}
}

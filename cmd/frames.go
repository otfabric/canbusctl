// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/capture"
	"github.com/otfabric/go-canbus/socketcan/link"
	"github.com/spf13/cobra"
)

// bounded returns ctx limited to d when d > 0.
func bounded(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d > 0 {
		return context.WithTimeout(ctx, d)
	}
	return context.WithCancel(ctx)
}

// ended reports an error that ends a stream normally: the duration
// elapsed or the user interrupted. go-canbus reports a socket deadline
// that fires a moment before ctx's own timer as context.DeadlineExceeded.
func ended(ctx context.Context, err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

type frameRecord struct {
	cli.FrameJSON
	Message string            `json:"message,omitempty"`
	Signals []decodedSignal   `json:"signals,omitempty"`
	ErrInfo *canbus.ErrorInfo `json:"errorInfo,omitempty"`
	Decode  string            `json:"decodeError,omitempty"`
}

type decodedSignal struct {
	Name     string `json:"name"`
	Raw      string `json:"raw"`
	Physical any    `json:"physical"` // a number, or "NaN", "+Inf", "-Inf" (JSON has no such numbers)
	Unit     string `json:"unit,omitempty"`
	Label    string `json:"label,omitempty"`
	value    float64
}

func decodeSignals(sigs []candb.DecodedSignal) []decodedSignal {
	out := make([]decodedSignal, 0, len(sigs))
	for _, s := range sigs {
		var phys any = s.Physical
		if math.IsNaN(s.Physical) || math.IsInf(s.Physical, 0) {
			phys = strconv.FormatFloat(s.Physical, 'g', -1, 64)
		}
		out = append(out, decodedSignal{Name: s.Signal.Name, Raw: s.Signal.RawString(s.Value), Physical: phys, Unit: s.Signal.Unit, Label: s.Label, value: s.Physical})
	}
	return out
}

// human formats a physical value for people: at most 12 significant
// digits, so 0.1 × 3 prints 0.3, not 0.30000000000000004. JSON output keeps
// the full float64 and the exact raw value.
func human(f float64) string { return strconv.FormatFloat(f, 'g', 12, 64) }

func signalsText(sigs []decodedSignal) string {
	var parts []string
	for _, s := range sigs {
		p := s.Name + "=" + human(s.value)
		if s.Unit != "" {
			p += " " + s.Unit
		}
		if s.Label != "" {
			p += " (" + s.Label + ")"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

// record builds the output of one frame, decoding it with db if given.
func record(f *canbus.Frame, db *candb.Database) (frameRecord, string) {
	r := frameRecord{FrameJSON: cli.ToJSON(f)}
	text := cli.FrameLine(f)
	if f.IsError() {
		if info, ok := canbus.ParseError(f); ok {
			r.ErrInfo = &info
		}
	}
	if db != nil && !f.IsError() && !f.IsRemote() {
		if m := db.Message(f.ID, f.IsExtended()); m != nil {
			r.Message = m.Name
			sigs, err := m.Decode(f.Payload())
			if err != nil {
				r.Decode = err.Error()
				text += "  " + m.Name + ": " + err.Error()
			} else {
				r.Signals = decodeSignals(sigs)
				text += "  " + m.Name + ": " + signalsText(r.Signals)
			}
		}
	}
	return r, text
}

type summary struct {
	Frames  uint64 `json:"frames"`
	Dropped uint64 `json:"dropped"`
}

func newMonitorCmd(a *app) *cobra.Command {
	var (
		iface, dbPath   string
		output          string
		filters         []string
		join, errFrames bool
		noFD            bool
		count           int
		duration        time.Duration
	)
	c := &cobra.Command{
		Use:   "monitor",
		Short: "Print the frames of an interface as they arrive (passive)",
		Long: `monitor receives frames and prints them, one line or JSON record each, until
--count frames, --duration or Ctrl-C. It never transmits. --dbc decodes frames of
known messages with a DBC, KCD or ARXML database. --output also writes the
frames to a trace file (format by extension, as record). Filters use candump notation:
<id>:<mask> or <id>~<mask> (inverted), hexadecimal, with ",std", ",ext", ",rtr"
or ",data"; several filters are ORed, or ANDed with --join.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			fs, err := cli.ParseFilters(filters, join, errFrames)
			if err != nil {
				return err
			}
			var db *candb.Database
			if dbPath != "" {
				if db, _, _, err = cli.LoadDatabase(dbPath); err != nil {
					return err
				}
			}
			bus, err := openBus(iface, openOpts{autoFD: !noFD, filters: &fs, passive: true})
			if err != nil {
				return err
			}
			defer func() { _ = bus.Close() }()
			sub, err := bus.Subscribe(canbus.WithQueueSize(65536))
			if err != nil {
				return err
			}
			var tw capture.Writer
			if output != "" {
				format, err := cli.TraceFormat(output, "")
				if err != nil {
					return err
				}
				file, err := os.Create(output)
				if err != nil {
					return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot create %s: %w", output, err)
				}
				defer func() { _ = file.Close() }()
				if tw, err = capture.NewWriter(file, format); err != nil {
					return err
				}
				defer func() { _ = tw.Close() }()
			}
			ctx, cancel := bounded(cmd.Context(), duration)
			defer cancel()
			st := a.stream()
			n := 0
			for count <= 0 || n < count {
				f, err := sub.Receive(ctx)
				if err != nil {
					if ended(ctx, err) {
						break
					}
					return err
				}
				if tw != nil {
					if err := tw.Write(&f); err != nil {
						return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", output, err)
					}
				}
				r, text := record(&f, db)
				if err := st.Record("frame", text, r); err != nil {
					return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
				}
				n++
			}
			s := summary{Frames: uint64(n), Dropped: bus.Stats().Dropped}
			if a.format == "json" {
				return st.Record("summary", "", s)
			}
			a.warn("%d frames, %d dropped", s.Frames, s.Dropped)
			return nil
		},
	}
	f := c.Flags()
	f.StringVarP(&iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	f.StringArrayVar(&filters, "filter", nil, "Filter <id>:<mask> (repeatable)")
	f.BoolVar(&join, "join", false, "A frame must pass all filters (CAN_RAW_JOIN_FILTERS)")
	f.BoolVar(&errFrames, "errors", false, "Also receive error frames")
	f.BoolVar(&noFD, "no-fd", false, "Do not open the socket for CAN FD even when the interface carries it")
	f.StringVar(&dbPath, "dbc", "", "Decode with this DBC, KCD or ARXML database")
	f.StringVarP(&output, "output", "o", "", "Also write the frames to this trace file")
	f.IntVarP(&count, "count", "n", 0, "Stop after this many frames (0: no limit)")
	f.DurationVar(&duration, "duration", 0, "Stop after this time (0: until Ctrl-C)")
	_ = c.MarkFlagRequired("interface")
	return c
}

// sendLimits are the safety bounds of transmitting commands.
type sendLimits struct {
	rate      float64
	maxFrames int
	duration  time.Duration
}

func addLimitFlags(c *cobra.Command, l *sendLimits, defRate float64) {
	c.Flags().Float64Var(&l.rate, "rate", defRate, "Transmit at most this many frames per second (0: no limit)")
	c.Flags().IntVar(&l.maxFrames, "max-frames", 10000, "Transmit at most this many frames")
	c.Flags().DurationVar(&l.duration, "duration", 0, "Stop transmitting after this time (0: no limit)")
}

type sendResult struct {
	Interface string          `json:"interface"`
	Sent      int             `json:"sent"`
	Frames    []cli.FrameJSON `json:"frames"`
}

func (r sendResult) Text() string {
	var b strings.Builder
	for i := range r.Frames {
		f := r.Frames[i]
		fmt.Fprintf(&b, "%s %s\n", r.Interface, frameText(f))
	}
	fmt.Fprintf(&b, "sent %d frame(s)", r.Sent)
	return b.String()
}

func frameText(f cli.FrameJSON) string {
	s := f.IDHex + "#"
	switch {
	case f.FD:
		nib := 0
		if f.BRS {
			nib |= 1
		}
		if f.ESI {
			nib |= 2
		}
		s += "#" + strconv.Itoa(nib) + strings.ToUpper(f.Data)
	case f.Remote:
		s += "R"
		if f.Len > 0 {
			s += strconv.Itoa(f.Len)
		}
	default:
		s += strings.ToUpper(f.Data)
	}
	return s
}

func newSendCmd(a *app, fd bool) *cobra.Command {
	var (
		iface         string
		yes           bool
		id, data      string
		extended, rtr bool
		brs, esi      bool
		dlc           int
		repeat        int
		gap           time.Duration
		limits        sendLimits
	)
	name, short := "send", "Transmit classical CAN frames"
	if fd {
		name, short = "send-fd", "Transmit CAN FD frames"
	}
	c := &cobra.Command{
		Use:   name + " [frame...]",
		Short: short + " (needs --yes on a non-virtual interface)",
		Long: `Frames are given in cansend notation: 123#DEADBEEF, 1234ABCD#11.22, 123#R,
123#R4, and for CAN FD 123##1<data> (the digit after ## is the flags nibble:
1 = BRS, 2 = ESI). Without arguments, --id and --data build one frame.
--repeat sends the frames that many times, --gap waits between frames.
On any interface that is not vcan, vxcan or "virtual" nothing is sent
without --yes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var frames []canbus.Frame
			for _, s := range args {
				f, err := cli.ParseFrame(s)
				if err != nil {
					return err
				}
				frames = append(frames, f)
			}
			if len(args) == 0 {
				if id == "" {
					return errs.Invalid("give frames as arguments, or --id and --data")
				}
				cid, err := cli.ParseID(id, extended)
				if err != nil {
					return err
				}
				payload, err := parseHexData(data)
				if err != nil {
					return err
				}
				var f canbus.Frame
				switch {
				case rtr:
					f, err = canbus.NewRemote(cid, extended, uint8(dlc))
				case fd:
					f, err = canbus.NewFD(cid, extended, brs, payload)
					if esi {
						f.Flags |= canbus.FlagESI
					}
				case extended:
					f, err = canbus.NewExtended(cid, payload)
				default:
					f, err = canbus.NewStandard(cid, payload)
				}
				if err != nil {
					return err
				}
				frames = append(frames, f)
			}
			for i := range frames {
				if frames[i].IsFD() != fd {
					if fd {
						return errs.Invalid("%s is a classical frame: use send", frames[i].String())
					}
					return errs.Invalid("%s is a CAN FD frame: use send-fd", frames[i].String())
				}
			}
			if repeat < 1 {
				return errs.Invalid("--repeat %d: at least 1", repeat)
			}
			total := repeat * len(frames)
			if limits.maxFrames > 0 && total > limits.maxFrames {
				return errs.Invalid("%d frames exceed --max-frames %d", total, limits.maxFrames)
			}
			if err := cli.TransmitCheck(iface, yes, name); err != nil {
				return err
			}
			t, err := openTransport(iface, openOpts{fd: fd})
			if err != nil {
				return err
			}
			defer func() { _ = t.Close() }()
			ctx, cancel := bounded(cmd.Context(), limits.duration)
			defer cancel()
			pace := newPacer(limits.rate, gap)
			res := sendResult{Interface: iface}
			for r := 0; r < repeat; r++ {
				for i := range frames {
					if err := pace.wait(ctx); err != nil {
						if ended(ctx, err) {
							return a.emit(name, res)
						}
						return err
					}
					if err := t.Send(ctx, &frames[i]); err != nil {
						if ended(ctx, err) { // --duration ended during a send
							return a.emit(name, res)
						}
						return err
					}
					res.Sent++
					if r == 0 {
						res.Frames = append(res.Frames, cli.ToJSON(&frames[i]))
					}
				}
			}
			return a.emit(name, res)
		},
	}
	fl := c.Flags()
	fl.StringVarP(&iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	fl.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	fl.StringVar(&id, "id", "", "Identifier in hexadecimal (with --data)")
	fl.StringVar(&data, "data", "", "Payload in hexadecimal")
	fl.BoolVar(&extended, "extended", false, "29-bit identifier")
	fl.IntVar(&repeat, "repeat", 1, "Send the frames this many times")
	fl.DurationVar(&gap, "gap", 0, "Pause between frames")
	if fd {
		fl.BoolVar(&brs, "brs", false, "Bit rate switching")
		fl.BoolVar(&esi, "esi", false, "Set the error state indicator")
	} else {
		fl.BoolVar(&rtr, "rtr", false, "Remote transmission request")
		fl.IntVar(&dlc, "dlc", 0, "Requested length of a remote frame")
	}
	addLimitFlags(c, &limits, 0)
	_ = c.MarkFlagRequired("interface")
	return c
}

func parseHexData(s string) ([]byte, error) {
	s = strings.NewReplacer(".", "", " ", "", ":", "").Replace(s)
	if len(s)%2 != 0 {
		return nil, errs.Invalid("--data %q: an even number of hex digits", s)
	}
	out := make([]byte, len(s)/2)
	for i := range out {
		v, err := strconv.ParseUint(s[2*i:2*i+2], 16, 8)
		if err != nil {
			return nil, errs.Invalid("--data %q: not hexadecimal", s)
		}
		out[i] = byte(v)
	}
	return out, nil
}

// pacer spaces transmissions: at least gap apart, at most rate per second.
type pacer struct {
	interval time.Duration
	last     time.Time
}

func newPacer(rate float64, gap time.Duration) *pacer {
	p := &pacer{interval: gap}
	if rate > 0 {
		if r := time.Duration(float64(time.Second) / rate); r > p.interval {
			p.interval = r
		}
	}
	return p
}

func (p *pacer) wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.interval > 0 && !p.last.IsZero() {
		if d := time.Until(p.last.Add(p.interval)); d > 0 {
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				return ctx.Err()
			case <-t.C:
			}
		}
	}
	p.last = time.Now()
	return nil
}

type recordResult struct {
	Interface string `json:"interface"`
	Output    string `json:"output"`
	Format    string `json:"format"`
	Frames    int    `json:"frames"`
	Dropped   uint64 `json:"dropped"`
}

func (r recordResult) Text() string {
	return fmt.Sprintf("recorded %d frames from %s to %s (%s), %d dropped", r.Frames, r.Interface, r.Output, r.Format, r.Dropped)
}

func newRecordCmd(a *app) *cobra.Command {
	var (
		iface, out, ff  string
		filters         []string
		errFrames, noFD bool
		count           int
		duration        time.Duration
	)
	c := &cobra.Command{
		Use:   "record",
		Short: "Record frames to a candump log, JSON Lines, pcap, ASC, BLF or TRC file (passive)",
		Long: `record writes received frames to --output, in the format of its extension
(.log/.txt candump, .jsonl lossless JSON Lines, .pcap Wireshark, .asc and
.blf Vector, .trc PEAK; ASC/BLF/TRC times are local wall-clock times: set TZ)
or --file-format,
until --count, --duration or Ctrl-C. candump logs keep microseconds; JSON Lines
keeps every field, including nanosecond timestamps.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := cli.TraceFormat(out, ff)
			if err != nil {
				return err
			}
			fs, err := cli.ParseFilters(filters, false, errFrames)
			if err != nil {
				return err
			}
			bus, err := openBus(iface, openOpts{autoFD: !noFD, filters: &fs, passive: true})
			if err != nil {
				return err
			}
			defer func() { _ = bus.Close() }()
			sub, err := bus.Subscribe(canbus.WithQueueSize(65536))
			if err != nil {
				return err
			}
			file, err := os.Create(out)
			if err != nil {
				return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot create %s: %w", out, err)
			}
			defer func() { _ = file.Close() }()
			w, err := capture.NewWriter(file, format)
			if err != nil {
				return err
			}
			ctx, cancel := bounded(cmd.Context(), duration)
			defer cancel()
			n, err := capture.RecordFrames(ctx, sub, w, count)
			if cerr := w.Close(); err == nil && cerr != nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			return a.emit("record", recordResult{Interface: iface, Output: out, Format: string(format), Frames: n, Dropped: bus.Stats().Dropped})
		},
	}
	f := c.Flags()
	f.StringVarP(&iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	f.StringVarP(&out, "output", "o", "", "Output file (required)")
	f.StringVar(&ff, "file-format", "", "candump, jsonl, pcap, asc, blf or trc (default: from the extension)")
	f.StringArrayVar(&filters, "filter", nil, "Filter <id>:<mask> (repeatable)")
	f.BoolVar(&errFrames, "errors", false, "Also record error frames")
	f.BoolVar(&noFD, "no-fd", false, "Do not open the socket for CAN FD")
	f.IntVarP(&count, "count", "n", 0, "Stop after this many frames")
	f.DurationVar(&duration, "duration", 0, "Stop after this time")
	_ = c.MarkFlagRequired("interface")
	_ = c.MarkFlagRequired("output")
	return c
}

type replayResult struct {
	Interface string  `json:"interface"`
	Input     string  `json:"input"`
	Read      int     `json:"read"`
	Sent      int     `json:"sent"`
	Filtered  int     `json:"filtered"`
	Skipped   int     `json:"skipped"`
	ElapsedS  float64 `json:"elapsedSeconds"`
	Limited   bool    `json:"limited"`
}

func (r replayResult) Text() string {
	s := fmt.Sprintf("replayed %d of %d frames from %s on %s in %.3f s", r.Sent, r.Read, r.Input, r.Interface, r.ElapsedS)
	if r.Filtered > 0 || r.Skipped > 0 {
		s += fmt.Sprintf(" (%d filtered, %d error frames skipped)", r.Filtered, r.Skipped)
	}
	if r.Limited {
		s += "; stopped by a limit"
	}
	return s
}

func newReplayCmd(a *app) *cobra.Command {
	var (
		iface    string
		yes      bool
		speed    float64
		maxSpeed bool
		filters  []string
		printF   bool
		limits   sendLimits
	)
	c := &cobra.Command{
		Use:   "replay <trace>",
		Short: "Transmit a recorded trace (needs --yes on a non-virtual interface)",
		Long: `replay sends the frames of a candump log, JSON Lines, pcap/pcapng, ASC or BLF file ("-":
stdin) with their recorded timing (--speed 1), scaled (--speed 2 is twice as fast)
or as fast as allowed (--max-speed). Error frames are never sent. --max-frames,
--duration and --rate bound the transmission. The trace is streamed, not loaded.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if speed <= 0 && !maxSpeed {
				return errs.Invalid("--speed %v: positive (or use --max-speed)", speed)
			}
			fs, err := cli.ParseFilters(filters, false, false)
			if err != nil {
				return err
			}
			if err := cli.TransmitCheck(iface, yes, "replay"); err != nil {
				return err
			}
			r, closeTrace, err := cli.OpenTrace(args[0], a.in)
			if err != nil {
				return err
			}
			defer closeTrace()
			t, err := openTransport(iface, openOpts{autoFD: true})
			if err != nil {
				return err
			}
			defer func() { _ = t.Close() }()
			opts := capture.ReplayOptions{Speed: speed, MaxFrames: limits.maxFrames, MaxDuration: limits.duration, Rate: limits.rate}
			if maxSpeed {
				opts.Speed = 0
			}
			if len(filters) > 0 {
				opts.Filter = &fs
			}
			st := a.stream()
			if printF {
				opts.OnFrame = func(f *canbus.Frame) {
					out := *f
					out.Interface, out.Timestamp = iface, time.Now()
					_ = st.Record("frame", cli.FrameLine(&out), cli.ToJSON(&out))
				}
			}
			stats, err := capture.Replay(cmd.Context(), r, t, opts)
			if err != nil && !ended(cmd.Context(), err) {
				return err
			}
			res := replayResult{Interface: iface, Input: args[0], Read: stats.Read, Sent: stats.Sent, Filtered: stats.Filtered,
				Skipped: stats.Skipped, ElapsedS: stats.Elapsed.Seconds(), Limited: stats.Limited}
			if printF && a.format == "json" {
				return st.Record("summary", "", res)
			}
			return a.emit("replay", res)
		},
	}
	f := c.Flags()
	f.StringVarP(&iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	f.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	f.Float64Var(&speed, "speed", 1, "Timing factor: 1 original, 2 twice as fast")
	f.BoolVar(&maxSpeed, "max-speed", false, "Ignore the recorded timing")
	f.StringArrayVar(&filters, "filter", nil, "Replay only frames passing <id>:<mask> (repeatable)")
	f.BoolVar(&printF, "print", false, "Print every frame sent")
	addLimitFlags(c, &limits, 0)
	_ = c.MarkFlagRequired("interface")
	return c
}

type idStat struct {
	ID     string  `json:"id"`
	Frames uint64  `json:"frames"`
	RateHz float64 `json:"rateHz"`
}

type statsResult struct {
	Interface string          `json:"interface"`
	Link      *link.Interface `json:"link,omitempty"`
	Live      *liveStats      `json:"live,omitempty"`
}

type liveStats struct {
	DurationS   float64  `json:"durationSeconds"`
	Frames      uint64   `json:"frames"`
	Bytes       uint64   `json:"bytes"`
	ErrorFrames uint64   `json:"errorFrames"`
	Dropped     uint64   `json:"dropped"`
	IDs         []idStat `json:"ids"`
}

func (r statsResult) Text() string {
	var b strings.Builder
	if r.Link != nil {
		b.WriteString(infoResult{*r.Link}.Text())
	}
	if l := r.Live; l != nil {
		fmt.Fprintf(&b, "live %.1f s: %d frames, %d bytes, %d error frames, %d dropped\n", l.DurationS, l.Frames, l.Bytes, l.ErrorFrames, l.Dropped)
		for _, s := range l.IDs {
			fmt.Fprintf(&b, "  %-9s %8d frames %10.2f Hz\n", s.ID, s.Frames, s.RateHz)
		}
	}
	return b.String()
}

func (r statsResult) Headers() []string { return []string{"ID", "FRAMES", "RATE_HZ"} }
func (r statsResult) Rows() [][]string {
	var rows [][]string
	if r.Live != nil {
		for _, s := range r.Live.IDs {
			rows = append(rows, []string{s.ID, strconv.FormatUint(s.Frames, 10), strconv.FormatFloat(s.RateHz, 'f', 2, 64)})
		}
	}
	return rows
}

func newStatsCmd(a *app) *cobra.Command {
	var (
		iface    string
		duration time.Duration
	)
	c := &cobra.Command{
		Use:   "stats",
		Short: "Show interface counters and, with --duration, count live traffic per identifier (passive)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res := statsResult{Interface: iface}
			if iface != cli.Virtual {
				it, err := link.Get(iface)
				if err != nil && duration == 0 {
					return err
				}
				if err == nil {
					res.Link = &it
				}
			}
			if duration > 0 {
				bus, err := openBus(iface, openOpts{autoFD: true, filters: &canbus.FilterSet{ErrorMask: canbus.ErrClassAll}, passive: true})
				if err != nil {
					return err
				}
				defer func() { _ = bus.Close() }()
				sub, _ := bus.Subscribe(canbus.WithQueueSize(65536))
				ctx, cancel := bounded(cmd.Context(), duration)
				defer cancel()
				start := time.Now()
				counts := map[string]uint64{}
				for {
					f, err := sub.Receive(ctx)
					if err != nil {
						if ended(ctx, err) {
							break
						}
						return err
					}
					if !f.IsError() {
						counts[cli.ToJSON(&f).IDHex]++
					}
				}
				el := time.Since(start).Seconds()
				bs := bus.Stats()
				l := &liveStats{DurationS: el, Frames: bs.RxFrames, Bytes: bs.RxBytes, ErrorFrames: bs.RxErrors, Dropped: bs.Dropped, IDs: []idStat{}}
				for id, n := range counts {
					l.IDs = append(l.IDs, idStat{ID: id, Frames: n, RateHz: float64(n) / el})
				}
				sort.Slice(l.IDs, func(i, j int) bool { return l.IDs[i].ID < l.IDs[j].ID })
				res.Live = l
			}
			return a.emit("stats", res)
		},
	}
	c.Flags().StringVarP(&iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	c.Flags().DurationVar(&duration, "duration", 0, "Count live traffic for this long")
	_ = c.MarkFlagRequired("interface")
	return c
}

// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
	"github.com/otfabric/go-canbus/socketcan"
)

// xlJSON is the JSON form of a CAN XL frame.
type xlJSON struct {
	Kind        string `json:"kind"` // "xl"
	Interface   string `json:"interface,omitempty"`
	Timestamp   string `json:"timestamp,omitempty"`
	Priority    uint16 `json:"priority"`
	PriorityHex string `json:"priorityHex"`
	VCID        uint8  `json:"vcid"`
	SEC         bool   `json:"sec,omitempty"`
	RRS         bool   `json:"rrs,omitempty"`
	SDT         uint8  `json:"sdt"`
	AF          uint32 `json:"af"`
	Len         int    `json:"len"`
	Data        string `json:"data"`
	Echo        bool   `json:"echo,omitempty"`
}

func toXLJSON(x *canbus.XLFrame) xlJSON {
	j := xlJSON{Kind: "xl", Interface: x.Interface, Priority: x.Priority, PriorityHex: fmt.Sprintf("%03X", x.Priority),
		VCID: x.VCID, SEC: x.Flags&canbus.XLSEC != 0, RRS: x.Flags&canbus.XLRRS != 0, SDT: x.SDT, AF: x.AF,
		Len: len(x.Data), Data: hex.EncodeToString(x.Data), Echo: x.Echo}
	if !x.Timestamp.IsZero() {
		j.Timestamp = cli.Timestamp(x.Timestamp)
	}
	return j
}

// xlLine is the text form of a received CAN XL frame (candump -L style).
func xlLine(x *canbus.XLFrame) string {
	iface := x.Interface
	if iface == "" {
		iface = "-"
	}
	ts := "(-)"
	if !x.Timestamp.IsZero() {
		ts = "(" + cli.Timestamp(x.Timestamp) + ")"
	}
	return ts + " " + iface + " " + x.String()
}

// openXL opens a SocketCAN socket in CAN XL mode. CAN XL exists only on
// SocketCAN: not on the virtual bus or slcan adapters.
func openXL(iface string, recvOwn bool) (*socketcan.Conn, error) {
	if iface == cli.Virtual || strings.HasPrefix(iface, cli.SlcanPrefix) {
		return nil, errs.New(errs.KindUnsupported, errs.CodeUnsupported, "%s: CAN XL needs a SocketCAN interface (Linux ≥ 6.2, mtu 2060)", iface)
	}
	opts := []socketcan.Option{socketcan.WithXL()}
	if recvOwn {
		opts = append(opts, socketcan.WithRecvOwn())
	}
	return socketcan.Open(iface, opts...)
}

func newXLCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "xl",
		Short: "CAN XL frames on SocketCAN (Linux ≥ 6.2, interface mtu 2060)",
		Long: `CAN XL frames (ISO 11898-1:2024: 11-bit priority, VCID, SEC and RRS flags,
SDU type, acceptance field, 1..2048 data bytes) on a SocketCAN interface with
the CAN XL MTU (ip link set vcanxl0 mtu 2060; Linux 6.2 or later, VCIDs 6.9).
Frames use can-utils notation: <vcid><prio>#<flags>:<sdt>:<af>#<data>, for
example 00242#80:03:00C0FFEE#11223344, or <prio>#... with VCID 0.`,
	}
	c.AddCommand(newXLSendCmd(a), newXLMonitorCmd(a))
	return c
}

type xlSendResult struct {
	Interface string   `json:"interface"`
	Sent      int      `json:"sent"`
	Frames    []xlJSON `json:"frames"`
}

func (r xlSendResult) Text() string {
	var b strings.Builder
	for i := range r.Frames {
		fmt.Fprintf(&b, "%s %s\n", r.Interface, r.Frames[i].text())
	}
	fmt.Fprintf(&b, "sent %d CAN XL frame(s)", r.Sent)
	return b.String()
}

func (j xlJSON) text() string {
	fl := 0x80
	if j.SEC {
		fl |= 0x01
	}
	if j.RRS {
		fl |= 0x02
	}
	return fmt.Sprintf("%02X%s#%02X:%02X:%08X#%s", j.VCID, j.PriorityHex, fl, j.SDT, j.AF, strings.ToUpper(j.Data))
}

func newXLSendCmd(a *app) *cobra.Command {
	var (
		iface  string
		yes    bool
		repeat int
		gap    time.Duration
		limits sendLimits
	)
	c := &cobra.Command{
		Use:   "send frame...",
		Short: "Transmit CAN XL frames (needs --yes on a non-virtual interface)",
		Long: `Frames in can-utils notation, as cansend takes them:
<vcid><prio>#<flags>:<sdt>:<af>#<data> (data bytes may be separated by dots).
--repeat sends the frames that many times, --gap waits between frames. On any
interface that is not vcan or vxcan nothing is sent without --yes.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			frames := make([]canbus.XLFrame, 0, len(args))
			for _, s := range args {
				x, err := canbus.ParseXLFrame(s)
				if err != nil {
					return errs.Invalid("%v", err)
				}
				frames = append(frames, x)
			}
			if repeat < 1 {
				return errs.Invalid("--repeat %d: at least 1", repeat)
			}
			if total := repeat * len(frames); limits.maxFrames > 0 && total > limits.maxFrames {
				return errs.Invalid("%d frames exceed --max-frames %d", total, limits.maxFrames)
			}
			if err := cli.TransmitCheck(iface, yes, "xl send"); err != nil {
				return err
			}
			conn, err := openXL(iface, false)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()
			ctx, cancel := bounded(cmd.Context(), limits.duration)
			defer cancel()
			pace := newPacer(limits.rate, gap)
			res := xlSendResult{Interface: iface}
			for r := 0; r < repeat; r++ {
				for i := range frames {
					if err := pace.wait(ctx); err != nil {
						if ended(ctx, err) {
							return a.emit("xl send", res)
						}
						return err
					}
					if err := conn.SendXL(ctx, &frames[i]); err != nil {
						if ended(ctx, err) {
							return a.emit("xl send", res)
						}
						return err
					}
					res.Sent++
					if r == 0 {
						res.Frames = append(res.Frames, toXLJSON(&frames[i]))
					}
				}
			}
			return a.emit("xl send", res)
		},
	}
	fl := c.Flags()
	fl.StringVarP(&iface, "interface", "i", "", "SocketCAN interface with the CAN XL MTU (required)")
	fl.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	fl.IntVar(&repeat, "repeat", 1, "Send the frames this many times")
	fl.DurationVar(&gap, "gap", 0, "Pause between frames")
	addLimitFlags(c, &limits, 0)
	_ = c.MarkFlagRequired("interface")
	return c
}

func newXLMonitorCmd(a *app) *cobra.Command {
	var (
		iface, output string
		count         int
		duration      time.Duration
	)
	c := &cobra.Command{
		Use:   "monitor",
		Short: "Print CAN, CAN FD and CAN XL frames as they arrive (passive)",
		Long: `xl monitor receives every frame of a CAN XL interface, CAN XL and CAN or
CAN FD alike, and prints them until --count frames, --duration or Ctrl-C. It
never transmits. --output also writes them to a candump log (.log) or JSON
Lines (.jsonl) trace, the formats that carry CAN XL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var tw capture.XLWriter
			if output != "" {
				format, err := cli.TraceFormat(output, "")
				if err != nil {
					return err
				}
				if format != capture.FormatCandump && format != capture.FormatJSONL {
					return errs.Invalid("--output %s: CAN XL is written to candump logs (.log) and JSON Lines (.jsonl) only", output)
				}
				file, err := os.Create(output)
				if err != nil {
					return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot create %s: %w", output, err)
				}
				defer func() { _ = file.Close() }()
				w, err := capture.NewWriter(file, format)
				if err != nil {
					return err
				}
				tw = w.(capture.XLWriter)
				defer func() { _ = tw.Close() }()
			}
			conn, err := openXL(iface, false)
			if err != nil {
				return err
			}
			defer func() { _ = conn.Close() }()
			ctx, cancel := bounded(cmd.Context(), duration)
			defer cancel()
			st := a.stream()
			var f canbus.Frame
			var x canbus.XLFrame
			n, xls := 0, 0
			for count <= 0 || n < count {
				isXL, err := conn.ReceiveAny(ctx, &f, &x)
				if err != nil {
					if ended(ctx, err) {
						break
					}
					return err
				}
				var werr error
				if isXL {
					xls++
					if tw != nil {
						werr = tw.WriteXL(&x)
					}
					if err := st.Record("frame", xlLine(&x), toXLJSON(&x)); err != nil {
						return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
					}
				} else {
					if tw != nil {
						werr = tw.Write(&f)
					}
					r, text := record(&f, nil)
					if err := st.Record("frame", text, r); err != nil {
						return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
					}
				}
				if werr != nil {
					return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write %s: %w", output, werr)
				}
				n++
			}
			s := xlSummary{Frames: n, XL: xls}
			if st, ok := any(conn).(canbus.Statser); ok {
				s.Dropped = st.Stats().Dropped
			}
			if a.format == "json" {
				return st.Record("summary", "", s)
			}
			a.warn("%d frames (%d CAN XL), %d dropped", s.Frames, s.XL, s.Dropped)
			return nil
		},
	}
	fl := c.Flags()
	fl.StringVarP(&iface, "interface", "i", "", "SocketCAN interface with the CAN XL MTU (required)")
	fl.StringVarP(&output, "output", "o", "", "Also write the frames to this trace file (.log or .jsonl)")
	fl.IntVarP(&count, "count", "n", 0, "Stop after this many frames (0: no limit)")
	fl.DurationVar(&duration, "duration", 0, "Stop after this time (0: until Ctrl-C)")
	_ = c.MarkFlagRequired("interface")
	return c
}

type xlSummary struct {
	Frames  int    `json:"frames"`
	XL      int    `json:"xl"`
	Dropped uint64 `json:"dropped"`
}

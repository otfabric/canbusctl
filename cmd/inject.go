// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/chaos"
	"github.com/otfabric/go-canbus/virtual"
	"github.com/spf13/cobra"
)

type injectResult struct {
	Interface  string      `json:"interface"`
	Source     int         `json:"sourceFrames"`
	Delivered  int         `json:"delivered"`
	Faults     chaos.Stats `json:"faults"`
	ErrorFrame string      `json:"errorFrame,omitempty"`
	Malformed  int         `json:"malformed"`
}

func (r injectResult) Text() string {
	s := fmt.Sprintf("%d source frames, %d delivered to %s: %d dropped, %d duplicated, %d delayed, %d reordered, %d corrupted",
		r.Source, r.Delivered, r.Interface, r.Faults.Dropped, r.Faults.Duplicated, r.Faults.Delayed, r.Faults.Reordered, r.Faults.Corrupted)
	if r.ErrorFrame != "" {
		s += "; error frame " + r.ErrorFrame
	}
	if r.Malformed > 0 {
		s += fmt.Sprintf("; %d malformed frames", r.Malformed)
	}
	return s
}

var errorClasses = map[string]canbus.ErrorInfo{
	"bus-off":   {Class: canbus.ErrClassBusOff},
	"passive":   {Class: canbus.ErrClassController, Controller: canbus.CtrlRxPassive | canbus.CtrlTxPassive},
	"warning":   {Class: canbus.ErrClassController, Controller: canbus.CtrlRxWarning | canbus.CtrlTxWarning},
	"no-ack":    {Class: canbus.ErrClassNoAck},
	"stuff":     {Class: canbus.ErrClassProtocol, Protocol: canbus.ProtStuff},
	"overflow":  {Class: canbus.ErrClassController, Controller: canbus.CtrlRxOverflow},
	"restarted": {Class: canbus.ErrClassRestarted},
}

func newInjectCmd(a *app) *cobra.Command {
	var (
		iface, trace, errClass string
		frames                 []string
		faults                 faultFlags
		malformed              int
		yes                    bool
		limits                 sendLimits
	)
	c := &cobra.Command{
		Use:   "inject",
		Short: "Pass traffic through controlled faults onto a virtual test bus",
		Long: `inject takes frames (--frame in cansend notation, or a --trace file) through
a seeded fault stage (the chaos bridge of go-canbus, as in "bridge") —
--drop, --duplicate, --delay-probability with --delay, --reorder, --corrupt
(one payload bit flipped) — and delivers what survives:

  --interface virtual  the frames are printed as a receiver on the in-process
                       virtual bus sees them; --error-frame (bus-off, passive,
                       warning, no-ack, stuff, overflow, restarted) and
                       --malformed N (frames violating the length rules)
                       are injected there too.
  vcan / vxcan         the surviving frames are transmitted, faults applied in
                       userspace before sending. Error frames and malformed
                       frames cannot be sent through SocketCAN (exit 8).
  other interfaces     refused without --yes.

All faults are emulated in userspace. Arbitration, bit errors, CRC failures,
error confinement and electrical faults need real hardware and are not
produced. The same --seed gives the same faults.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := faults.config(nil)
			if err != nil {
				return err
			}
			var info *canbus.ErrorInfo
			if errClass != "" {
				i, ok := errorClasses[errClass]
				if !ok {
					return errs.Invalid("--error-frame %q: one of bus-off, passive, warning, no-ack, stuff, overflow, restarted", errClass)
				}
				info = &i
			}
			if iface != cli.Virtual && (info != nil || malformed > 0) {
				return errs.New(errs.KindUnsupported, errs.CodeUnsupported,
					"error frames and malformed frames exist only on the in-process virtual bus: SocketCAN cannot send them")
			}
			var src []canbus.Frame
			for _, s := range frames {
				f, err := cli.ParseFrame(s)
				if err != nil {
					return err
				}
				src = append(src, f)
			}
			if trace != "" {
				r, closer, err := cli.OpenTrace(trace, a.in)
				if err != nil {
					return err
				}
				defer closer()
				for {
					var f canbus.Frame
					if err := r.Read(&f); err != nil {
						break
					}
					if !f.IsError() {
						f.Timestamp, f.Interface, f.Echo = time.Time{}, "", false
						src = append(src, f)
					}
					if limits.maxFrames > 0 && len(src) > limits.maxFrames {
						return errs.Invalid("the trace has more than --max-frames %d frames", limits.maxFrames)
					}
				}
			}
			if len(src) == 0 && info == nil && malformed == 0 {
				return errs.Invalid("nothing to inject: give --frame, --trace, --error-frame or --malformed")
			}
			if err := cli.TransmitCheck(iface, yes, "inject"); err != nil {
				return err
			}
			// The fault stage: a chaos bridge between two private virtual
			// buses; error and malformed frames are injected after it.
			srcBus := virtual.New(virtual.Config{Name: cli.Virtual})
			defer func() { _ = srcBus.Close() }()
			dstBus := virtual.New(virtual.Config{Name: cli.Virtual})
			defer func() { _ = dstBus.Close() }()
			in, _ := srcBus.Open(virtual.WithFD())
			bridgeIn, _ := srcBus.Open(virtual.WithFD(), virtual.WithQueueSize(len(src)+16))
			bridgeOut, _ := dstBus.Open(virtual.WithFD())
			outPort, _ := dstBus.Open(virtual.WithFD(), virtual.WithQueueSize(len(src)*2+malformed+16),
				virtual.WithFilters(canbus.FilterSet{ErrorMask: canbus.ErrClassAll}))
			br, err := chaos.New(cfg)
			if err != nil {
				return errs.Invalid("%v", err)
			}
			var target canbus.Transport
			if iface != cli.Virtual {
				t, err := openTransport(iface, openOpts{autoFD: true})
				if err != nil {
					return err
				}
				defer func() { _ = t.Close() }()
				target = t
			}
			ctx, cancel := bounded(cmd.Context(), limits.duration)
			defer cancel()
			bridgeDone := make(chan struct{})
			go func() { _ = br.Run(ctx, bridgeIn, bridgeOut); close(bridgeDone) }()
			for i := range src {
				if err := in.Send(ctx, &src[i]); err != nil {
					_ = bridgeIn.Close()
					<-bridgeDone
					return err
				}
			}
			// Once the bridge has taken every source frame, closing its
			// input ends it after the frame in hand (a reordered frame is
			// flushed then).
			for br.Stats().Received < uint64(len(src)) && ctx.Err() == nil {
				time.Sleep(time.Millisecond)
			}
			_ = bridgeIn.Close()
			<-bridgeDone
			res := injectResult{Interface: iface, Source: len(src)}
			if info != nil {
				if err := dstBus.InjectError(ctx, *info); err != nil {
					return err
				}
				res.ErrorFrame = errClass
			}
			for i := 0; i < malformed; i++ {
				bad := canbus.Frame{ID: canbus.ID(0x700 + i%0x100), Len: uint8(9 + i%7)} // classical with 9..15 bytes
				if i%2 == 1 {
					bad = canbus.Frame{ID: 0x7FF, Flags: canbus.FlagFD, Len: 13} // not a CAN FD length
				}
				if err := dstBus.InjectRaw(ctx, bad); err != nil {
					return err
				}
				res.Malformed++
			}
			st := a.stream()
			pace := newPacer(limits.rate, 0)
			for {
				var f canbus.Frame
				c, stop := context.WithTimeout(ctx, 100*time.Millisecond)
				err := outPort.Receive(c, &f)
				stop()
				if err != nil {
					break
				}
				res.Delivered++
				if target != nil {
					if err := pace.wait(ctx); err != nil {
						break
					}
					out := f
					out.Timestamp, out.Interface, out.Echo = time.Time{}, "", false
					if err := target.Send(ctx, &out); err != nil {
						return err
					}
					continue
				}
				f.Interface = iface
				text := cli.FrameLine(&f)
				if f.IsError() {
					text = strings.Replace(text, f.String(), f.String()+" (error frame)", 1)
				} else if f.Validate() != nil {
					text += "  MALFORMED"
				}
				_ = st.Record("frame", text, cli.ToJSON(&f))
			}
			res.Faults = br.Stats()
			return a.emit("inject", res)
		},
	}
	f := c.Flags()
	f.StringVarP(&iface, "interface", "i", cli.Virtual, "\"virtual\", a vcan/vxcan interface, or another with --yes")
	f.StringArrayVar(&frames, "frame", nil, "A frame in cansend notation (repeatable)")
	f.StringVar(&trace, "trace", "", "Frames from a trace file")
	f.StringVar(&errClass, "error-frame", "", "Inject an error frame (virtual bus only)")
	f.IntVar(&malformed, "malformed", 0, "Inject this many malformed frames (virtual bus only)")
	f.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	addLimitFlags(c, &limits, 0)
	faults.add(c)
	return c
}

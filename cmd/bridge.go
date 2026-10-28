// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"sync"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/chaos"
	"github.com/spf13/cobra"
)

// faultFlags are the chaos.Config flags shared by bridge and inject.
type faultFlags struct {
	drop, dup, delayP, reorder, corrupt float64
	delay                               time.Duration
	seed                                int64
}

func (f *faultFlags) add(c *cobra.Command) {
	fl := c.Flags()
	fl.Float64Var(&f.drop, "drop", 0, "Probability a frame is lost")
	fl.Float64Var(&f.dup, "duplicate", 0, "Probability a frame is forwarded twice")
	fl.Float64Var(&f.delayP, "delay-probability", 0, "Probability a frame is held for --delay (later frames wait behind it)")
	fl.DurationVar(&f.delay, "delay", 10*time.Millisecond, "Hold time of delayed frames")
	fl.Float64Var(&f.reorder, "reorder", 0, "Probability a frame is sent after the next one")
	fl.Float64Var(&f.corrupt, "corrupt", 0, "Probability one payload bit is flipped")
	fl.Int64Var(&f.seed, "seed", 1, "Seed of the fault generator")
}

func (f *faultFlags) config(filter *canbus.FilterSet) (chaos.Config, error) {
	for _, p := range []float64{f.drop, f.dup, f.delayP, f.reorder, f.corrupt} {
		if p < 0 || p > 1 {
			return chaos.Config{}, errs.Invalid("fault probabilities lie between 0 and 1")
		}
	}
	return chaos.Config{Seed: f.seed, Drop: f.drop, Duplicate: f.dup, Delay: f.delayP, DelayBy: f.delay,
		Reorder: f.reorder, Corrupt: f.corrupt, Filter: filter}, nil
}

type bridgeResult struct {
	A         string      `json:"a"`
	B         string      `json:"b"`
	OneWay    bool        `json:"oneWay,omitempty"`
	DurationS float64     `json:"durationSeconds"`
	AToB      chaos.Stats `json:"aToB"`
	BToA      chaos.Stats `json:"bToA"`
}

func statsText(s chaos.Stats) string {
	return fmt.Sprintf("%d received, %d forwarded: %d dropped, %d duplicated, %d delayed, %d reordered, %d corrupted",
		s.Received, s.Forwarded, s.Dropped, s.Duplicated, s.Delayed, s.Reordered, s.Corrupted)
}

func (r bridgeResult) Text() string {
	s := fmt.Sprintf("bridged %s and %s for %.1f s\n  %s → %s: %s", r.A, r.B, r.DurationS, r.A, r.B, statsText(r.AToB))
	if !r.OneWay {
		s += fmt.Sprintf("\n  %s → %s: %s", r.B, r.A, statsText(r.BToA))
	}
	return s
}

func newBridgeCmd(a *app) *cobra.Command {
	var (
		ifA, ifB string
		filters  []string
		oneWay   bool
		yes      bool
		duration time.Duration
		faults   faultFlags
	)
	c := &cobra.Command{
		Use:   "bridge",
		Short: "Forward frames between two interfaces through seeded faults (needs --yes on a non-virtual interface)",
		Long: `bridge forwards every frame received on -a to -b and, unless --one-way, from
-b to -a, through a seeded fault stage: --drop, --duplicate,
--delay-probability with --delay, --reorder, --corrupt. --filter limits the
faults to matching frames (others pass untouched). Peers connected to only
one side see each other through the bridge: put it between vcan0 and vcan1
to test a stack, or a device on a bench bus, under faults.

Interfaces: vcan/vxcan, "virtual" (the in-process bus), or others with --yes
(the bridge transmits). Error frames and a socket's own transmissions are not
forwarded. Runs for --duration (required) or until Ctrl-C, then prints the
counters per direction.

The faults are userspace emulation of what a receiver sees. Corrupted frames
are delivered, where a real bus would reject them by CRC; a single bus never
reorders a sender's frames. The same --seed gives the same decisions for the
same traffic.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if ifA == ifB {
				return errs.Invalid("-a and -b must be different interfaces")
			}
			if duration <= 0 {
				return errs.Invalid("--duration is required (the bridge transmits)")
			}
			var fs *canbus.FilterSet
			if len(filters) > 0 {
				set, err := cli.ParseFilters(filters, false, false)
				if err != nil {
					return err
				}
				fs = &set
			}
			cfg, err := faults.config(fs)
			if err != nil {
				return err
			}
			if err := cli.TransmitCheck(ifB, yes, "bridge"); err != nil {
				return err
			}
			if !oneWay {
				if err := cli.TransmitCheck(ifA, yes, "bridge"); err != nil {
					return err
				}
			}
			ta, err := openTransport(ifA, openOpts{autoFD: true})
			if err != nil {
				return err
			}
			defer func() { _ = ta.Close() }()
			tb, err := openTransport(ifB, openOpts{autoFD: true})
			if err != nil {
				return err
			}
			defer func() { _ = tb.Close() }()
			ab, err := chaos.New(cfg)
			if err != nil {
				return errs.Invalid("%v", err)
			}
			cfgBA := cfg
			cfgBA.Seed++ // independent decisions per direction
			ba, _ := chaos.New(cfgBA)
			ctx, cancel := bounded(cmd.Context(), duration)
			defer cancel()
			start := time.Now()
			var wg sync.WaitGroup
			var mu sync.Mutex
			var first error
			run := func(br *chaos.Bridge, src, dst canbus.Transport) {
				defer wg.Done()
				if err := br.Run(ctx, src, dst); err != nil { // Run returns nil when ctx ends
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
					cancel()
				}
			}
			wg.Add(1)
			go run(ab, ta, tb)
			if !oneWay {
				wg.Add(1)
				go run(ba, tb, ta)
			}
			<-ctx.Done()
			// Close the transports to end blocked receives, then wait.
			_ = ta.Close()
			_ = tb.Close()
			wg.Wait()
			if first != nil {
				return first
			}
			return a.emit("bridge", bridgeResult{A: ifA, B: ifB, OneWay: oneWay, DurationS: time.Since(start).Seconds(),
				AToB: ab.Stats(), BToA: ba.Stats()})
		},
	}
	f := c.Flags()
	f.StringVarP(&ifA, "side-a", "a", "", "First interface (required)")
	f.StringVarP(&ifB, "side-b", "b", "", "Second interface (required)")
	f.BoolVar(&oneWay, "one-way", false, "Forward from -a to -b only")
	f.StringArrayVar(&filters, "filter", nil, "Apply faults only to frames matching <id>:<mask> (repeatable)")
	f.DurationVar(&duration, "duration", 0, "Run for this long (required)")
	f.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	faults.add(c)
	_ = c.MarkFlagRequired("side-a")
	_ = c.MarkFlagRequired("side-b")
	return c
}

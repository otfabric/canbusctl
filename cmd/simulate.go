// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/candb/bits"
	"github.com/spf13/cobra"
)

// generator produces deterministic signal values for a message.
type generator struct {
	m       *candb.Message
	pattern string
	rng     *rand.Rand
	step    uint64
}

func (g *generator) raw(s *candb.Signal) uint64 {
	switch g.pattern {
	case "initial":
		if s.Initial != nil {
			return bits.Truncate(uint64(*s.Initial), s.Length)
		}
		return 0
	case "random":
		return bits.Truncate(g.rng.Uint64(), s.Length)
	}
	// counter: the step number, wrapping at the field width; floats count
	// in physical units.
	switch s.ValueType {
	case candb.Float32:
		return uint64(math.Float32bits(float32(g.step)))
	case candb.Float64:
		return math.Float64bits(float64(g.step))
	default:
	}
	return bits.Truncate(g.step, s.Length)
}

// next returns the values of the next frame: every signal the chosen
// multiplexer values select.
func (g *generator) next() map[string]candb.Input {
	vals := map[string]uint64{}
	active := map[string]bool{}
	var visit func(s *candb.Signal) bool
	visit = func(s *candb.Signal) bool {
		if on, done := active[s.Name]; done {
			return on
		}
		on := true
		if s.Multiplexer != "" {
			sw := g.m.Signal(s.Multiplexer)
			on = sw != nil && visit(sw) && s.ActiveFor(vals[sw.Name])
		}
		active[s.Name] = on
		if on {
			r := g.raw(s)
			if s.IsMultiplexer {
				r = g.muxValue(s)
			}
			vals[s.Name] = r
		}
		return on
	}
	for _, s := range g.m.Signals {
		visit(s)
	}
	in := map[string]candb.Input{}
	for name, r := range vals {
		in[name] = candb.Raw(r)
	}
	g.step++
	return in
}

// muxValue cycles a switch through the values its signals use.
func (g *generator) muxValue(sw *candb.Signal) uint64 {
	var vs []uint64
	for _, s := range g.m.Signals {
		if s.Multiplexer == sw.Name {
			for _, r := range s.MuxValues {
				vs = append(vs, r.Low)
			}
		}
	}
	if len(vs) == 0 {
		return 0
	}
	return vs[g.step%uint64(len(vs))]
}

type simResult struct {
	Interface string         `json:"interface"`
	Sent      int            `json:"sent"`
	PerMsg    map[string]int `json:"messages"`
	ElapsedS  float64        `json:"elapsedSeconds"`
}

func (r simResult) Text() string {
	return fmt.Sprintf("simulated %d frames of %d messages on %s in %.2f s", r.Sent, len(r.PerMsg), r.Interface, r.ElapsedS)
}

func newSimulateCmd(a *app) *cobra.Command {
	var (
		iface, dbPath, pattern string
		msgs                   []string
		seed                   int64
		defCycle               time.Duration
		yes, printF            bool
		limits                 sendLimits
	)
	c := &cobra.Command{
		Use:   "simulate",
		Short: "Generate deterministic traffic of a database's messages at their cycle times",
		Long: `simulate sends the messages of a DBC or KCD database at their cycle times
(GenMsgCycleTime / interval; --default-cycle for the others) with signal values
from --pattern: counter (the step number), random (seeded by --seed) or initial.
The same seed and database give the same frames. With --interface virtual the
traffic stays in this process and is printed; on a vcan or vxcan interface it
is sent; on any other interface it needs --yes. --duration (default 10s),
--max-frames and --rate bound it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, _, _, err := cli.LoadDatabase(dbPath)
			if err != nil {
				return err
			}
			if pattern != "counter" && pattern != "random" && pattern != "initial" {
				return errs.Invalid("--pattern %q: counter, random or initial", pattern)
			}
			if limits.duration <= 0 {
				return errs.Invalid("--duration must be positive: simulation is always bounded")
			}
			type job struct {
				g      *generator
				period time.Duration
				due    time.Duration
			}
			rng := rand.New(rand.NewSource(seed))
			var jobs []*job
			for _, m := range db.Messages {
				if len(msgs) > 0 && !contains(msgs, m.Name) {
					continue
				}
				p := time.Duration(m.CycleTime) * time.Millisecond
				if p <= 0 {
					p = defCycle
				}
				if p <= 0 {
					continue
				}
				jobs = append(jobs, &job{g: &generator{m: m, pattern: pattern, rng: rng}, period: p})
			}
			if len(jobs) == 0 {
				return errs.Invalid("no message to simulate (cycle times are 0: give --default-cycle)")
			}
			if err := cli.TransmitCheck(iface, yes, "simulate"); err != nil {
				return err
			}
			t, err := openTransport(iface, openOpts{autoFD: true})
			if err != nil {
				return err
			}
			defer func() { _ = t.Close() }()
			ctx, cancel := bounded(cmd.Context(), limits.duration)
			defer cancel()
			pace := newPacer(limits.rate, 0)
			st := a.stream()
			show := printF || iface == cli.Virtual
			res := simResult{Interface: iface, PerMsg: map[string]int{}}
			start := time.Now()
			for limits.maxFrames <= 0 || res.Sent < limits.maxFrames {
				j := jobs[0]
				for _, o := range jobs[1:] {
					if o.due < j.due {
						j = o
					}
				}
				if d := time.Until(start.Add(j.due)); d > 0 {
					timer := time.NewTimer(d)
					select {
					case <-ctx.Done():
						timer.Stop()
					case <-timer.C:
					}
				}
				if ctx.Err() != nil {
					break
				}
				f, err := j.g.m.Frame(j.g.next(), candb.EncodeOptions{Lax: true})
				if err != nil {
					return err
				}
				if err := pace.wait(ctx); err != nil {
					break
				}
				if err := t.Send(ctx, &f); err != nil {
					if ctx.Err() != nil {
						break
					}
					return err
				}
				res.Sent++
				res.PerMsg[j.g.m.Name]++
				j.due += j.period
				if show {
					out := f
					out.Interface, out.Timestamp = iface, time.Now()
					r, text := record(&out, db)
					_ = st.Record("frame", text, r)
				}
			}
			res.ElapsedS = time.Since(start).Seconds()
			if show && a.format == "json" {
				return st.Record("summary", "", res)
			}
			return a.emit("simulate", res)
		},
	}
	f := c.Flags()
	f.StringVarP(&iface, "interface", "i", cli.Virtual, "CAN interface, or \"virtual\" (in this process)")
	f.StringVar(&dbPath, "dbc", "", "DBC or KCD database (required)")
	f.StringVar(&pattern, "pattern", "counter", "Signal values: counter, random or initial")
	f.StringSliceVar(&msgs, "message", nil, "Simulate only these messages (repeatable)")
	f.Int64Var(&seed, "seed", 1, "Seed of the random pattern")
	f.DurationVar(&defCycle, "default-cycle", 0, "Period of messages without a cycle time (0: skip them)")
	f.BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	f.BoolVar(&printF, "print", false, "Print every frame (always on for the virtual bus)")
	addLimitFlags(c, &limits, 0)
	_ = f.Set("duration", "10s")
	f.Lookup("duration").DefValue = "10s"
	_ = c.MarkFlagRequired("dbc")
	return c
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

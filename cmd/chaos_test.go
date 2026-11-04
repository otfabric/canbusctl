// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/bits"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/chaos"
	"github.com/otfabric/go-canbus/virtual"
)

// chaosIterations is the number of seeds of the sweeps below
// ($CANBUS_CHAOS_ITERATIONS, default def).
func chaosIterations(def int) int {
	if n, err := strconv.Atoi(os.Getenv("CANBUS_CHAOS_ITERATIONS")); err == nil && n > 0 {
		return n
	}
	return def
}

// faultSet is one combination of --drop, --duplicate, ... flags. ordered
// is set when the fault set promises to keep the order of the frames
// (no --reorder: a delayed frame holds the ones behind it).
type faultSet struct {
	name    string
	flags   []string
	ordered bool
	corrupt bool
}

var chaosFaultSets = []faultSet{
	{"drop+dup", []string{"--drop", "0.2", "--duplicate", "0.2"}, true, false},
	{"delay+drop+dup", []string{"--delay-probability", "0.3", "--delay", "1ms", "--drop", "0.1", "--duplicate", "0.1"}, true, false},
	{"all", []string{"--drop", "0.1", "--duplicate", "0.1", "--delay-probability", "0.1", "--delay", "1ms", "--reorder", "0.3", "--corrupt", "0.3"}, false, true},
}

// decodeAll reads every JSON value of s (JSON Lines records and an
// indented result document), failing on anything that is not JSON.
func decodeAll(t testing.TB, s string) []map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	var out []map[string]any
	for {
		var m map[string]any
		if err := dec.Decode(&m); err == io.EOF {
			return out
		} else if err != nil {
			t.Fatalf("output is not JSON: %v\n%s", err, s)
		}
		out = append(out, m)
	}
}

// chaosFrame is a delivered frame: its identifier and payload.
type chaosFrame struct {
	id   uint32
	data []byte
}

// sourceFrames are n frames with increasing identifiers 0x100.. and a
// payload naming their index.
func sourceFrames(n int) []canbus.Frame {
	out := make([]canbus.Frame, n)
	for i := range out {
		out[i], _ = canbus.NewStandard(canbus.ID(0x100+i), []byte{byte(i), 0xA5, byte(i * 7)})
	}
	return out
}

// checkDelivered asserts the invariants of frames that went through the
// fault stage: no frame invented (every identifier was sent), each
// delivered at most twice (a duplicate), payloads intact unless --corrupt
// (then at most one bit flipped), and, when the set promises it, the
// order kept.
func checkDelivered(t *testing.T, fs faultSet, src []canbus.Frame, got []chaosFrame) {
	t.Helper()
	count := map[uint32]int{}
	last := uint32(0)
	for _, g := range got {
		i := int(g.id) - 0x100
		if i < 0 || i >= len(src) {
			t.Fatalf("%s: frame %03X invented", fs.name, g.id)
		}
		count[g.id]++
		if count[g.id] > 2 {
			t.Fatalf("%s: frame %03X delivered %d times", fs.name, g.id, count[g.id])
		}
		want := src[i].Payload()
		if len(g.data) != len(want) {
			t.Fatalf("%s: frame %03X length %d, sent %d", fs.name, g.id, len(g.data), len(want))
		}
		flipped := 0
		for k := range want {
			flipped += bits.OnesCount8(g.data[k] ^ want[k])
		}
		if !fs.corrupt && flipped != 0 || flipped > 1 {
			t.Fatalf("%s: frame %03X payload % X, sent % X", fs.name, g.id, g.data, want)
		}
		if fs.ordered && g.id < last {
			t.Fatalf("%s: frame %03X after %03X: order lost", fs.name, g.id, last)
		}
		last = g.id
	}
}

func statsOf(t *testing.T, m any) chaos.Stats {
	t.Helper()
	b, _ := json.Marshal(m)
	var s chaos.Stats
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

// checkStats asserts the counters add up: every frame received is
// forwarded once, unless dropped, plus once more per duplicate.
func checkStats(t *testing.T, what string, s chaos.Stats, received int) {
	t.Helper()
	if s.Received != uint64(received) {
		t.Fatalf("%s: %d received, %d sent: %+v", what, s.Received, received, s)
	}
	if s.Forwarded != s.Received-s.Dropped+s.Duplicated {
		t.Fatalf("%s: counters do not add up: %+v", what, s)
	}
}

// goroutines returns the stacks of the running goroutines by id.
func goroutines() map[string]string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	out := map[string]string{}
	for _, g := range strings.Split(string(buf), "\n\n") {
		if f := strings.Fields(g); len(f) >= 2 && f[0] == "goroutine" {
			out[f[1]] = g
		}
	}
	return out
}

// noLeak fails when goroutines started after before are still running
// 3 s later (the shared virtual bus is closed first: it outlives a
// command by design).
func noLeak(t *testing.T, before map[string]string) {
	t.Helper()
	resetVirtual()
	var leaked []string
	for end := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		leaked = leaked[:0]
		for id, g := range goroutines() {
			if _, ok := before[id]; !ok && !strings.Contains(g, "testing.tRunner") && !strings.Contains(g, "runtime.Stack") {
				leaked = append(leaked, g)
			}
		}
		if len(leaked) == 0 || time.Now().After(end) {
			break
		}
	}
	if len(leaked) > 0 {
		t.Fatalf("%d goroutines leaked:\n%s", len(leaked), strings.Join(leaked, "\n\n"))
	}
}

// TestChaosInjectSeeds sweeps the seeds of inject on the virtual bus for
// every fault set. Invariants: the source frames are all received by the
// fault stage; forwarded = received - dropped + duplicated; delivered =
// forwarded + malformed + error frames, and as many frame records are
// printed; no frame is invented, none delivered more than twice, the
// payload is intact without --corrupt (at most one bit flipped with it),
// the order is kept without --reorder; the same seed gives the same
// output; no goroutine outlives the command. On failure the seed and
// fault flags are printed. Seeds: $CANBUS_CHAOS_ITERATIONS (default 6).
func TestChaosInjectSeeds(t *testing.T) {
	const n = 50
	src := sourceFrames(n)
	before := goroutines()
	var total faultTotals
	for _, fs := range chaosFaultSets {
		for seed := 1; seed <= chaosIterations(6); seed++ {
			args := []string{"inject", "--format", "json", "--seed", strconv.Itoa(seed)}
			for i := range src {
				args = append(args, "--frame", src[i].String())
			}
			args = append(args, fs.flags...)
			extra := 0
			if seed%2 == 0 { // error and malformed frames are injected after the fault stage
				args = append(args, "--error-frame", "bus-off", "--malformed", "3")
				extra = 4
			}
			t.Run(fmt.Sprintf("%s/seed=%d", fs.name, seed), func(t *testing.T) {
				r := run(t, "", args...)
				if r.code != 0 {
					t.Fatalf("%v: exit %d: %s", args[4:], r.code, r.errOut)
				}
				docs := decodeAll(t, r.out)
				res := docs[len(docs)-1]["result"].(map[string]any)
				st := statsOf(t, res["faults"])
				checkStats(t, fmt.Sprintf("seed %d %v", seed, fs.flags), st, n)
				total.add(st)
				if res["sourceFrames"].(float64) != n {
					t.Fatalf("source %v", res["sourceFrames"])
				}
				var got []chaosFrame
				records := 0
				for _, d := range docs[:len(docs)-1] {
					if d["type"] != "frame" {
						continue
					}
					records++
					id := uint32(d["id"].(float64))
					if d["error"] == true || id >= 0x700 {
						continue
					}
					data, _ := hex.DecodeString(d["data"].(string))
					got = append(got, chaosFrame{id, data})
				}
				if del := int(res["delivered"].(float64)); del != records || del != int(st.Forwarded)+extra || len(got) != int(st.Forwarded) {
					t.Fatalf("seed %d %v: delivered %d, %d records, %d frames, forwarded %d, %d injected", seed, fs.flags, del, records, len(got), st.Forwarded, extra)
				}
				checkDelivered(t, fs, src, got)
				again := run(t, "", args...)
				if strings.Join(frameLines(again.out), "\n") != strings.Join(frameLines(r.out), "\n") {
					t.Fatalf("seed %d %v: the same seed gave different frames", seed, fs.flags)
				}
			})
		}
	}
	total.check(t)
	noLeak(t, before)
}

// frameLines returns the lines of a JSON output without timestamps.
func frameLines(out string) []string {
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		if strings.Contains(l, `"type":"frame"`) {
			var m map[string]any
			_ = json.Unmarshal([]byte(l), &m)
			delete(m, "timestamp")
			b, _ := json.Marshal(m)
			lines = append(lines, string(b))
		}
	}
	return lines
}

// chaosBuses replaces openTransport so that the interfaces "chaos-a" and
// "chaos-b" are two private virtual buses; opened is closed once the
// bridge has opened both.
type chaosBuses struct {
	a, b   *virtual.Bus
	mu     sync.Mutex
	n      int
	opened chan struct{}
}

func newChaosBuses(t *testing.T) *chaosBuses {
	t.Helper()
	cb := &chaosBuses{a: virtual.New(virtual.Config{Name: "chaos-a"}), b: virtual.New(virtual.Config{Name: "chaos-b"}), opened: make(chan struct{})}
	prev := openTransport
	openTransport = func(iface string, o openOpts) (canbus.Transport, error) {
		bus := map[string]*virtual.Bus{"chaos-a": cb.a, "chaos-b": cb.b}[iface]
		if bus == nil {
			return prev(iface, o)
		}
		p, err := bus.Open(virtual.WithFD(), virtual.WithQueueSize(4096))
		cb.mu.Lock()
		if cb.n++; cb.n == 2 {
			close(cb.opened)
		}
		cb.mu.Unlock()
		return p, err
	}
	t.Cleanup(func() {
		openTransport = prev
		_ = cb.a.Close()
		_ = cb.b.Close()
	})
	return cb
}

// drain receives frames of identifiers in [lo, lo+n) until the port has
// been idle for 150 ms.
func drain(t *testing.T, p *virtual.Port, lo uint32, n int) []chaosFrame {
	t.Helper()
	var got []chaosFrame
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		var f canbus.Frame
		err := p.Receive(ctx, &f)
		cancel()
		if err != nil {
			return got
		}
		if uint32(f.ID) >= lo && uint32(f.ID) < lo+uint32(n) {
			got = append(got, chaosFrame{uint32(f.ID) - lo + 0x100, slices.Clone(f.Payload())})
		}
	}
}

// bridgeRun runs one bridge between chaos-a and chaos-b, sending src on
// both sides (identifiers 0x100.. on a, 0x500.. on b), and returns its
// result and the frames each side received through it.
func bridgeRun(t *testing.T, src []canbus.Frame, args []string) (map[string]any, []chaosFrame, []chaosFrame, time.Duration) {
	t.Helper()
	cb := newChaosBuses(t)
	txA, _ := cb.a.Open(virtual.WithFD())
	rxA, _ := cb.a.Open(virtual.WithFD(), virtual.WithQueueSize(4096))
	txB, _ := cb.b.Open(virtual.WithFD())
	rxB, _ := cb.b.Open(virtual.WithFD(), virtual.WithQueueSize(4096))
	rc := make(chan result, 1)
	began := time.Now()
	go func() {
		var o, e bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code := Run(ctx, args, strings.NewReader(""), &o, &e)
		rc <- result{code, o.String(), e.String()}
	}()
	select {
	case <-cb.opened:
	case r := <-rc:
		t.Fatalf("bridge ended early: exit %d: %s%s", r.code, r.out, r.errOut)
	}
	ctx := context.Background()
	for i := range src {
		fa := src[i]
		fb := src[i]
		fb.ID += 0x400
		if err := txA.Send(ctx, &fa); err != nil {
			t.Fatal(err)
		}
		if err := txB.Send(ctx, &fb); err != nil {
			t.Fatal(err)
		}
	}
	r := <-rc
	elapsed := time.Since(began)
	if r.code != 0 {
		t.Fatalf("%v: exit %d: %s", args, r.code, r.errOut)
	}
	var atob, btoa []chaosFrame
	var wg sync.WaitGroup
	wg.Go(func() { atob = drain(t, rxB, 0x100, len(src)) })
	btoa = drain(t, rxA, 0x500, len(src))
	wg.Wait()
	return jsonDoc(t, r.out)["result"].(map[string]any), atob, btoa, elapsed
}

// faultTotals sums the counters of a sweep: each fault must have fired,
// or the sweep proves nothing.
type faultTotals struct{ chaos.Stats }

func (ft *faultTotals) add(s chaos.Stats) {
	ft.Dropped += s.Dropped
	ft.Duplicated += s.Duplicated
	ft.Delayed += s.Delayed
	ft.Reordered += s.Reordered
	ft.Corrupted += s.Corrupted
}

func (ft *faultTotals) check(t *testing.T) {
	t.Helper()
	if ft.Dropped == 0 || ft.Duplicated == 0 || ft.Delayed == 0 || ft.Reordered == 0 || ft.Corrupted == 0 {
		t.Fatalf("a fault never fired in the sweep: %+v", ft.Stats)
	}
}

// TestChaosBridgeSeeds sweeps the seeds of bridge between two virtual
// buses (in process, both directions) for every fault set. Invariants,
// per direction: every frame sent is received by the bridge; forwarded =
// received - dropped + duplicated and exactly that many frames arrive on
// the other side; no frame is invented, none arrives more than twice,
// payloads are intact without --corrupt, the order is kept without
// --reorder; the command returns within --duration + 2 s; the same seed
// gives the same frames; no goroutine outlives it. Seeds:
// $CANBUS_CHAOS_ITERATIONS (default 4).
func TestChaosBridgeSeeds(t *testing.T) {
	const n = 40
	src := sourceFrames(n)
	const duration = 200 * time.Millisecond
	before := goroutines()
	var total faultTotals
	for _, fs := range chaosFaultSets {
		for seed := 1; seed <= chaosIterations(3); seed++ {
			args := append([]string{"bridge", "-a", "chaos-a", "-b", "chaos-b", "--yes", "--duration", duration.String(),
				"--format", "json", "--seed", strconv.Itoa(seed)}, fs.flags...)
			t.Run(fmt.Sprintf("%s/seed=%d", fs.name, seed), func(t *testing.T) {
				res, atob, btoa, el := bridgeRun(t, src, args)
				if el > duration+2*time.Second {
					t.Fatalf("bridge took %v for --duration %v", el, duration)
				}
				for _, d := range []struct {
					name string
					key  string
					got  []chaosFrame
				}{{"a→b", "aToB", atob}, {"b→a", "bToA", btoa}} {
					st := statsOf(t, res[d.key])
					checkStats(t, fmt.Sprintf("%s seed %d %v", d.name, seed, fs.flags), st, n)
					total.add(st)
					if uint64(len(d.got)) != st.Forwarded {
						t.Fatalf("%s seed %d %v: %d frames arrived, %d forwarded", d.name, seed, fs.flags, len(d.got), st.Forwarded)
					}
					checkDelivered(t, fs, src, d.got)
				}
				if seed == 1 {
					_, again, _, _ := bridgeRun(t, src, args)
					if fmt.Sprint(again) != fmt.Sprint(atob) {
						t.Fatalf("seed %d %v: the same seed forwarded different frames", seed, fs.flags)
					}
				}
			})
		}
	}
	total.check(t)
	noLeak(t, before)
}

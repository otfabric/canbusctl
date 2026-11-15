// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
)

// lowerIDLimit sets the identifier bound of trace info and stats to n for
// the test.
func lowerIDLimit(t *testing.T, n int) {
	t.Helper()
	prev := idLimit
	idLimit = n
	t.Cleanup(func() { idLimit = prev })
}

// idTrace writes a candump log of the frames with the identifiers ids, in
// that order, one millisecond apart.
func idTrace(t *testing.T, ids []uint32) string {
	t.Helper()
	var b strings.Builder
	for i, id := range ids {
		width := 3
		if id > 0x7FF {
			width = 8
		}
		fmt.Fprintf(&b, "(%d.%06d) can0 %0*X#00\n", 1700000000+i/1000, i%1000*1000, width, id)
	}
	p := filepath.Join(t.TempDir(), "ids.log")
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// idCase is a sequence of identifiers with d distinct ones (0..d-1), the
// first three of them seen again after all d, and, when d is above the
// limit, the last one again.
func idCase(d, limit int) (ids []uint32, listed, other int) {
	for i := range d {
		ids = append(ids, uint32(i))
	}
	ids = append(ids, 0, 1, 2)
	listed, other = min(d, limit), max(0, d-limit)
	if d > limit {
		ids = append(ids, uint32(d-1))
		other++
	}
	return ids, listed, other
}

// TestTraceInfoIdentifierBound: trace info lists at most idLimit
// identifiers; the frames of further ones are counted in otherFrames, and
// the frames of a listed identifier are counted after the bound is
// reached. Boundary table around a lowered bound.
func TestTraceInfoIdentifierBound(t *testing.T) {
	const limit = 8
	lowerIDLimit(t, limit)
	for _, d := range []int{limit + 1, limit + 12, limit - 1, limit} {
		ids, listed, other := idCase(d, limit)
		p := idTrace(t, ids)
		r := run(t, "", "trace", "info", p, "--format", "json")
		want(t, r, 0)
		res := jsonDoc(t, r.out)["result"].(map[string]any)
		got := res["ids"].([]any)
		if len(got) != listed {
			t.Fatalf("%d identifiers: %d listed, want %d", d, len(got), listed)
		}
		if res["otherFrames"] != float64(other) {
			t.Fatalf("%d identifiers: otherFrames %v, want %d", d, res["otherFrames"], other)
		}
		sum := 0
		for _, g := range got {
			e := g.(map[string]any)
			sum += int(e["frames"].(float64))
			if e["id"] == "000" && e["frames"] != float64(2) {
				t.Fatalf("%d identifiers: 000 counted %v times, seen twice", d, e["frames"])
			}
		}
		if sum+other != len(ids) || res["frames"] != float64(len(ids)) {
			t.Fatalf("%d identifiers: %d listed + %d other frames, %v frames, %d in the trace", d, sum, other, res["frames"], len(ids))
		}
		text := run(t, "", "trace", "info", p)
		table := run(t, "", "trace", "info", p, "--format", "table")
		line := fmt.Sprintf("%d frames of further identifiers not listed (at most %d identifiers are listed)", other, limit)
		if other > 0 {
			want(t, text, 0, line)
			want(t, table, 0)
			if !strings.Contains(table.errOut, line) {
				t.Fatalf("table: the remainder is not reported on stderr: %q", table.errOut)
			}
		} else if strings.Contains(text.out+table.out+table.errOut, "further identifiers") {
			t.Fatalf("%d identifiers: a remainder reported:\n%s%s%s", d, text.out, table.out, table.errOut)
		}
	}
}

// TestTraceInfoIdentifierBoundDefault: the documented bound, 65536
// identifiers, with a trace of 1000 more extended identifiers.
func TestTraceInfoIdentifierBoundDefault(t *testing.T) {
	if testing.Short() {
		t.Skip("-short: 66536 frames")
	}
	if idLimit != 65536 || maxDistinctIDs != 65536 {
		t.Fatalf("the bound is %d (%d), documented as 65536", idLimit, maxDistinctIDs)
	}
	ids := make([]uint32, 65536+1000)
	for i := range ids {
		ids[i] = 0x10000000 + uint32(i)
	}
	r := run(t, "", "trace", "info", idTrace(t, ids), "--format", "json")
	want(t, r, 0)
	res := jsonDoc(t, r.out)["result"].(map[string]any)
	if n := len(res["ids"].([]any)); n != 65536 || res["otherFrames"] != float64(1000) || res["frames"] != float64(len(ids)) {
		t.Fatalf("%d listed, otherFrames %v, frames %v", n, res["otherFrames"], res["frames"])
	}
}

// feedTransport is a transport that yields its frames, then cancels the
// command (as Ctrl-C does) and waits to be closed. Receive is called by
// one goroutine, the Bus that reads it: when it asks for the frame after
// the last one, every frame has been handed to the subscriptions.
type feedTransport struct {
	frames []canbus.Frame
	next   int
	cancel context.CancelFunc
	closed chan struct{}
	once   sync.Once
}

func (s *feedTransport) Send(context.Context, *canbus.Frame) error { return canbus.ErrUnsupported }

func (s *feedTransport) Receive(ctx context.Context, f *canbus.Frame) error {
	if s.next < len(s.frames) {
		*f = s.frames[s.next]
		s.next++
		return nil
	}
	s.cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return canbus.ErrClosed
	}
}

func (s *feedTransport) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

// runOnSource runs a command whose interface "src0" yields frames; the
// command is cancelled once it has received them all.
func runOnSource(t *testing.T, frames []canbus.Frame, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	src := &feedTransport{frames: frames, cancel: cancel, closed: make(chan struct{})}
	prev := openTransport
	openTransport = func(iface string, o openOpts) (canbus.Transport, error) {
		if iface == "src0" {
			return src, nil
		}
		return prev(iface, o)
	}
	defer func() { openTransport = prev }()
	var out, errOut bytes.Buffer
	code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("%q did not end", args)
	}
	return result{code, out.String(), errOut.String()}
}

// TestStatsIdentifierBound: stats --duration lists at most idLimit
// identifiers, as trace info does. Boundary table around a lowered bound.
func TestStatsIdentifierBound(t *testing.T) {
	const limit = 8
	lowerIDLimit(t, limit)
	// stats divides by the time it counted: once it has started, let the
	// clock move, which a coarse clock (Windows) may not do in a run this
	// short.
	stats := func(frames []canbus.Frame, args ...string) result {
		t.Helper()
		onReady(t, "stats", func() {
			for t0 := time.Now(); time.Since(t0) <= 0; {
				runtime.Gosched()
			}
		})
		return runOnSource(t, frames, args...)
	}
	for _, d := range []int{limit + 1, limit + 12, limit - 1, limit} {
		ids, listed, other := idCase(d, limit)
		frames := make([]canbus.Frame, len(ids))
		for i, id := range ids {
			frames[i], _ = canbus.NewStandard(canbus.ID(id), []byte{byte(i)})
		}
		r := stats(frames, "stats", "-i", "src0", "--duration", "1m", "--format", "json")
		want(t, r, 0)
		live := jsonDoc(t, r.out)["result"].(map[string]any)["live"].(map[string]any)
		got := live["ids"].([]any)
		if len(got) != listed {
			t.Fatalf("%d identifiers: %d listed, want %d", d, len(got), listed)
		}
		if live["otherFrames"] != float64(other) || live["frames"] != float64(len(ids)) {
			t.Fatalf("%d identifiers: otherFrames %v, frames %v; want %d, %d", d, live["otherFrames"], live["frames"], other, len(ids))
		}
		for _, g := range got {
			if e := g.(map[string]any); e["id"] == "000" && e["frames"] != float64(2) {
				t.Fatalf("%d identifiers: 000 counted %v times, seen twice", d, e["frames"])
			}
		}
		line := fmt.Sprintf("%d frames of further identifiers not listed", other)
		text := stats(frames, "stats", "-i", "src0", "--duration", "1m")
		table := stats(frames, "stats", "-i", "src0", "--duration", "1m", "--format", "table")
		if other > 0 {
			want(t, text, 0, line)
			want(t, table, 0)
			if !strings.Contains(table.errOut, line) {
				t.Fatalf("table: the remainder is not reported on stderr: %q", table.errOut)
			}
		} else if strings.Contains(text.out+table.errOut, "further identifiers") {
			t.Fatalf("%d identifiers: a remainder reported:\n%s%s", d, text.out, table.errOut)
		}
	}
}

// frameSink is a transport that keeps what is sent to it; first is
// closed with the first frame.
type frameSink struct {
	mu     sync.Mutex
	sent   []canbus.Frame
	first  chan struct{}
	closed chan struct{}
	once   sync.Once
}

func (s *frameSink) Send(_ context.Context, f *canbus.Frame) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sent = append(s.sent, *f); len(s.sent) == 1 {
		close(s.first)
	}
	return nil
}

func (s *frameSink) Receive(ctx context.Context, _ *canbus.Frame) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return canbus.ErrClosed
	}
}

func (s *frameSink) Close() error {
	s.once.Do(func() { close(s.closed) })
	return nil
}

func (s *frameSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sent)
}

// withSink makes the interface "sink0" a frameSink.
func withSink(t *testing.T) *frameSink {
	t.Helper()
	s := &frameSink{first: make(chan struct{}), closed: make(chan struct{})}
	prev := openTransport
	openTransport = func(iface string, o openOpts) (canbus.Transport, error) {
		if iface == "sink0" {
			return s, nil
		}
		return prev(iface, o)
	}
	t.Cleanup(func() { openTransport = prev })
	return s
}

// TestInjectMaxFramesDuplicates: inject on an interface transmits at most
// --max-frames frames, the duplicates of the fault stage included (5
// source frames, each duplicated: 10 survive), and reports when the bound
// held frames back. It transmitted all 10.
func TestInjectMaxFramesDuplicates(t *testing.T) {
	base := []string{"inject", "-i", "sink0", "--yes", "--duplicate", "1"}
	for i := range 5 {
		base = append(base, "--frame", fmt.Sprintf("1%02X#%02X", i, i))
	}
	args := append(append([]string{}, base...), "--format", "json")
	for _, c := range []struct {
		max     string
		sent    int
		limited bool
	}{
		{"5", 5, true},
		{"6", 6, true},
		{"0", 10, false},
		{"9", 9, true},
		{"10", 10, false},
		{"11", 10, false},
	} {
		sink := withSink(t)
		r := run(t, "", append(append([]string{}, args...), "--max-frames", c.max)...)
		want(t, r, 0)
		docs := decodeAll(t, r.out)
		res := docs[len(docs)-1]["result"].(map[string]any)
		if n := sink.count(); n != c.sent {
			t.Fatalf("--max-frames %s: %d frames transmitted, want %d", c.max, n, c.sent)
		}
		if res["delivered"] != float64(c.sent) || res["limited"] != c.limited {
			t.Fatalf("--max-frames %s: delivered %v, limited %v; want %d, %v", c.max, res["delivered"], res["limited"], c.sent, c.limited)
		}
		if st := statsOf(t, res["faults"]); st.Duplicated != 5 {
			t.Fatalf("--max-frames %s: %d duplicated, want 5", c.max, st.Duplicated)
		}
		text := run(t, "", append(append([]string{}, base...), "--max-frames", c.max)...)
		if strings.Contains(text.out, "stopped by --max-frames") != c.limited {
			t.Fatalf("--max-frames %s: text %q", c.max, text.out)
		}
	}
	// The source alone above the bound is refused before anything is sent.
	sink := withSink(t)
	want(t, run(t, "", append(append([]string{}, args...), "--max-frames", "4")...), 3, "exceed --max-frames 4")
	if sink.count() != 0 {
		t.Fatal("frames transmitted")
	}
}

// TestDatabaseStreamed: a database file is streamed to the parser, which
// bounds it, instead of being read whole first: a 256 MiB KCD file is
// refused having allocated far less than its size (it allocated all of
// it).
func TestDatabaseStreamed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.kcd")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`<NetworkDefinition xmlns="http://kayak.2codeornot2code.org/1.0"><Bus name="Main"><`); err != nil {
		t.Fatal(err)
	}
	const size = 256 << 20
	if err := f.Truncate(size); err != nil { // the rest is zero bytes: no element name
		t.Fatal(err)
	}
	_ = f.Close()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	r := run(t, "", "dbc", "inspect", p, "--format", "json")
	runtime.ReadMemStats(&after)
	want(t, r, 3, `"code": "invalid_database"`)
	if d := after.TotalAlloc - before.TotalAlloc; d > 16<<20 {
		t.Fatalf("%d MiB allocated for a %d MiB file that is refused", d>>20, size>>20)
	}
}

// TestDatabaseKindByContent: without a known extension the kind is told by
// the content after any leading white space, as before the file was
// streamed: XML with an AUTOSAR root is ARXML, other XML is KCD, the rest
// DBC. The SHA-256 dbc generate records is that of the whole file.
func TestDatabaseKindByContent(t *testing.T) {
	dir := t.TempDir()
	read := func(p string) []byte {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, c := range []struct {
		src, kind string
		lead      int
	}{
		{"testdata/sample.kcd", "kcd", 0},
		{"testdata/sample.kcd", "kcd", 10000},
		{"testdata/system.arxml", "arxml", 0},
		{"testdata/system.arxml", "arxml", 5000},
		{"testdata/vehicle.dbc", "dbc", 0},
		{"testdata/vehicle.dbc", "dbc", 3},
	} {
		lead := bytes.Repeat([]byte(" \r\n\t"), c.lead/4+1)[:c.lead]
		p := filepath.Join(dir, fmt.Sprintf("db%d-%s", c.lead, c.kind))
		if err := os.WriteFile(p, append(lead, read(c.src)...), 0o600); err != nil {
			t.Fatal(err)
		}
		r := run(t, "", "dbc", "inspect", p, "--format", "json")
		want(t, r, 0)
		if got := jsonDoc(t, r.out)["result"].(map[string]any)["format"]; got != c.kind {
			t.Fatalf("%s with %d bytes of white space: %v, want %s", c.src, c.lead, got, c.kind)
		}
	}
	// A file of white space only is an empty DBC, as before.
	blank := filepath.Join(dir, "blank")
	if err := os.WriteFile(blank, []byte(" \n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	want(t, run(t, "", "dbc", "inspect", blank, "--format", "json"), 0, `"format": "dbc"`)
	// The parser of a KCD stops after the root element: the sum still
	// covers what follows it.
	kcd := filepath.Join(dir, "tail.kcd")
	if err := os.WriteFile(kcd, append(read("testdata/sample.kcd"), "\n<!-- after the root -->\n"...), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{vehicle, kcd} {
		sum := sha256.Sum256(read(p))
		want(t, run(t, "", "dbc", "generate", p, "--json"), 0, `"sourceSha256": "`+hex.EncodeToString(sum[:])+`"`)
	}
}

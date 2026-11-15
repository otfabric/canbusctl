// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/virtual"
)

var update = flag.Bool("update", false, "rewrite the golden files")

type result struct {
	code        int
	out, errOut string
}

// run executes canbusctl in process with a fresh virtual bus.
func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	resetVirtual()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code := Run(ctx, args, strings.NewReader(stdin), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

func resetVirtual() {
	virtualMu.Lock()
	defer virtualMu.Unlock()
	if virtualBus != nil {
		_ = virtualBus.Close()
	}
	virtualBus = nil
}

// runWithTraffic runs a command on the virtual bus while frames are sent
// to it repeatedly until the command returns.
func runWithTraffic(t *testing.T, frames []canbus.Frame, args ...string) result {
	t.Helper()
	resetVirtual()
	vb := sharedVirtual()
	port, err := vb.Open(virtual.WithFD())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for {
			for i := range frames {
				select {
				case <-done:
					return
				default:
				}
				_ = port.Send(context.Background(), &frames[i])
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
	close(done)
	return result{code, out.String(), errOut.String()}
}

// onReady sets readyHook for the test: fn runs once, in the command,
// when the command what is ready (it has opened its interface and waits).
func onReady(t *testing.T, what string, fn func()) {
	t.Helper()
	onReadyEach(t, map[string]func(){what: fn})
}

// onReadyEach is onReady for several commands: fns[what] runs once when
// the command what is ready.
func onReadyEach(t *testing.T, fns map[string]func()) {
	t.Helper()
	once := map[string]*sync.Once{}
	for what := range fns {
		once[what] = new(sync.Once)
	}
	h := func(w string) {
		if o := once[w]; o != nil {
			o.Do(fns[w])
		}
	}
	readyHook.Store(&h)
	t.Cleanup(func() { readyHook.Store(nil) })
}

// readySignals returns, per command name, a channel closed when that
// command is ready.
func readySignals(t *testing.T, names ...string) map[string]chan struct{} {
	t.Helper()
	chs := map[string]chan struct{}{}
	fns := map[string]func(){}
	for _, n := range names {
		ch := make(chan struct{})
		chs[n], fns[n] = ch, func() { close(ch) }
	}
	onReadyEach(t, fns)
	return chs
}

// await waits for a ready signal of a command running in the background,
// failing when the command ends first.
func await(t *testing.T, what string, ready <-chan struct{}, done <-chan result) {
	t.Helper()
	select {
	case <-ready:
	case r := <-done:
		t.Fatalf("%s ended before it was ready: exit %d\nstdout: %s\nstderr: %s", what, r.code, r.out, r.errOut)
	}
}

func want(t *testing.T, r result, code int, contains ...string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", r.code, code, r.out, r.errOut)
	}
	all := r.out + r.errOut
	for _, c := range contains {
		if !strings.Contains(all, c) {
			t.Fatalf("output lacks %q\nstdout: %s\nstderr: %s", c, r.out, r.errOut)
		}
	}
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./cmd -update)", err)
	}
	if strings.ReplaceAll(string(b), "\r\n", "\n") != got { // CRLF checkouts on Windows
		t.Fatalf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, b)
	}
}

func jsonDoc(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	return m
}

func jsonLines(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for l := range strings.SplitSeq(strings.TrimSpace(s), "\n") {
		if l != "" {
			out = append(out, jsonDoc(t, l))
		}
	}
	return out
}

func linux() bool { return runtime.GOOS == "linux" }

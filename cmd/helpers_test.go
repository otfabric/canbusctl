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
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l != "" {
			out = append(out, jsonDoc(t, l))
		}
	}
	return out
}

func linux() bool { return runtime.GOOS == "linux" }

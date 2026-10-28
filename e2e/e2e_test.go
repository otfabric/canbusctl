// SPDX-License-Identifier: MIT

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "canbusctl-e2e")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "canbusctl")
	if v := os.Getenv("CANBUSCTL_BIN"); v != "" {
		bin = v // a prebuilt binary (the microVM has no writable Go cache)
	} else if out, err := exec.Command("go", "build", "-race", "-o", bin, "..").CombinedOutput(); err != nil {
		// -race needs cgo; fall back to a plain build.
		if out2, err2 := exec.Command("go", "build", "-o", bin, "..").CombinedOutput(); err2 != nil {
			fmt.Fprintf(os.Stderr, "build: %v\n%s\n%s", err2, out, out2)
			os.Exit(2)
		}
	}
	// ASC and BLF hold zone-less local times.
	os.Setenv("TZ", "UTC")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	code     int
	out, err string
}

func run(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	r := result{out: o.String(), err: e.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return r
}

func want(t *testing.T, r result, code int, contains ...string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit %d, want %d\nstdout: %s\nstderr: %s", r.code, code, r.out, r.err)
	}
	for _, c := range contains {
		if !strings.Contains(r.out, c) {
			t.Fatalf("output lacks %q\nstdout: %s\nstderr: %s", c, r.out, r.err)
		}
	}
}

// decodeSignals returns the decoded signals of each frame of a trace.
func decodeSignals(t *testing.T, db, trace string) []string {
	t.Helper()
	r := run(t, "", "dbc", "decode", db, "--trace", trace, "--format", "json")
	if r.code != 0 && r.code != 1 {
		t.Fatalf("decode %s: exit %d: %s", trace, r.code, r.err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(r.out), "\n") {
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		out = append(out, string(m["idHex"])+string(m["data"])+string(m["signals"]))
	}
	return out
}

const vehicle = "../cmd/testdata/vehicle.dbc"

// TestTraceChain simulates traffic into a trace, converts it through
// every writable format and back, and decodes each with the same result.
func TestTraceChain(t *testing.T) {
	dir := t.TempDir()
	// The simulator on the in-process virtual bus prints its frames;
	// they become a candump log.
	r := run(t, "", "simulate", "--dbc", vehicle, "--max-frames", "40", "--format", "json", "--duration", "5s")
	want(t, r, 0)
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(r.out), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["type"] == "frame" {
			lines = append(lines, fmt.Sprintf("(1700000000.%06d) can0 %s#%s", len(lines)*1000, m["idHex"], strings.ToUpper(m["data"].(string))))
		}
	}
	if len(lines) < 20 {
		t.Fatalf("%d simulated frames: %s", len(lines), r.out)
	}
	src := filepath.Join(dir, "src.log")
	if err := os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := decodeSignals(t, vehicle, src)
	in := src
	for _, ext := range []string{".jsonl", ".pcap", ".asc", ".blf", ".log", ".blf", ".jsonl"} {
		out := filepath.Join(dir, fmt.Sprintf("c%d%s", len(ext)+len(in), ext))
		want(t, run(t, "", "trace", "convert", in, out), 0, fmt.Sprintf("%d frames read, %d written", len(lines), len(lines)))
		got := decodeSignals(t, vehicle, out)
		if strings.Join(got, "\n") != strings.Join(base, "\n") {
			t.Fatalf("%s decodes differently", out)
		}
		info := run(t, "", "trace", "info", out, "--format", "json")
		want(t, info, 0, fmt.Sprintf(`"frames": %d`, len(lines)))
		in = out
	}
	// The ARXML and KCD forms of a database decode alike.
	kcd := filepath.Join(dir, "v.kcd")
	want(t, run(t, "", "dbc", "convert", vehicle, "--to", "kcd", "-o", kcd), 0)
	if got := decodeSignals(t, kcd, in); strings.Join(got, "\n") != strings.Join(base, "\n") {
		t.Fatal("the KCD conversion decodes differently")
	}
	back := filepath.Join(dir, "v.dbc")
	want(t, run(t, "", "dbc", "convert", kcd, "--to", "dbc", "-o", back), 0)
	if got := decodeSignals(t, back, in); strings.Join(got, "\n") != strings.Join(base, "\n") {
		t.Fatal("the DBC → KCD → DBC round trip decodes differently")
	}
}

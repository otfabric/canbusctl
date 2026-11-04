// SPDX-License-Identifier: MIT

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHardeningExitCodes: the error behaviour of the SAST pass and of
// go-canbus v0.8.0, through the built binary.
func TestHardeningExitCodes(t *testing.T) {
	// --dlc outside 0..8 wrapped mod 256 (--dlc 260 sent DLC 4).
	for _, d := range []string{"-1", "9", "260"} {
		want(t, run(t, "", "send", "-i", "virtual", "--rtr", "--id", "123", "--dlc", d, "--format", "json"), 3, `"kind": "invalid_input"`)
	}
	want(t, run(t, "", "send", "-i", "virtual", "--rtr", "--id", "123", "--dlc", "8"), 0, "123#R8")
	// --repeat × frames overflowed past --max-frames.
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "4611686018427387904", "123#00", "123#00"), 3)
	want(t, run(t, "", "send", "-i", "virtual", "--rate", "-1", "123#00"), 3)

	dir := t.TempDir()
	// Output files are owner-only.
	outs := []string{filepath.Join(dir, "t.pcapng"), filepath.Join(dir, "g.go"), filepath.Join(dir, "v.kcd"), filepath.Join(dir, "r.log")}
	want(t, run(t, "", "trace", "convert", "../cmd/testdata/trace.log", outs[0]), 0)
	want(t, run(t, "", "dbc", "generate", vehicle, "-o", outs[1]), 0)
	want(t, run(t, "", "dbc", "convert", vehicle, "--to", "kcd", "-o", outs[2]), 0)
	want(t, run(t, "", "record", "-i", "virtual", "-o", outs[3], "--duration", "10ms"), 0)
	if runtime.GOOS != "windows" {
		for _, p := range outs {
			if st, err := os.Stat(p); err != nil || st.Mode().Perm() != 0o600 {
				t.Errorf("%s: %v %v", filepath.Base(p), st.Mode(), err)
			}
		}
	}

	// A 1960 trace cannot be written as pcap: exit 10 with the reason and
	// no file; BLF holds it.
	asc := filepath.Join(dir, "t.asc")
	want(t, run(t, "", "trace", "convert", "../cmd/testdata/trace.log", asc), 0)
	b, _ := os.ReadFile(asc)
	old := filepath.Join(dir, "old.asc")
	if err := os.WriteFile(old, []byte(strings.ReplaceAll(string(b), " 2023", " 1960")), 0o600); err != nil {
		t.Fatal(err)
	}
	pcap := filepath.Join(dir, "old.pcap")
	r := run(t, "", "trace", "convert", old, pcap, "--format", "json")
	want(t, r, 10, `"code": "output_failed"`, "not representable", "1970..2106")
	if _, err := os.Stat(pcap); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed conversion left %s", pcap)
	}
	want(t, run(t, "", "trace", "convert", old, filepath.Join(dir, "old.blf")), 0, "4 written")
	// A timestamp after 9999 does not read.
	far := filepath.Join(dir, "far.log")
	_ = os.WriteFile(far, []byte("(99999999999999.000000) can0 123#00\n"), 0o600)
	want(t, run(t, "", "trace", "info", far, "--format", "json"), 3, `"code": "invalid_capture"`)
	// Converting onto the input is refused, the input kept.
	want(t, run(t, "", "trace", "convert", far, far), 3)
	if b, _ := os.ReadFile(far); len(b) == 0 {
		t.Fatal("input truncated")
	}
}

// SPDX-License-Identifier: MIT

package cmd

import (
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSendRemoteDLCRange: a --dlc outside 0..8 is refused (it wrapped
// mod 256: --dlc 260 sent DLC 4).
func TestSendRemoteDLCRange(t *testing.T) {
	for _, d := range []string{"-1", "9", "16", "260"} {
		want(t, run(t, "", "send", "--interface", "virtual", "--rtr", "--id", "123", "--dlc", d), 3, "a remote frame requests 0 to 8 bytes")
	}
	want(t, run(t, "", "send", "--interface", "virtual", "--rtr", "--id", "123", "--dlc", "8"), 0, "sent 1 frame")
}

// TestOutputFilesOwnerOnly: traces and generated files are created 0600.
func TestOutputFilesOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX permissions")
	}
	dir := t.TempDir()
	trace := filepath.Join(dir, "t.pcapng")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", trace), 0, "written")
	gen := filepath.Join(dir, "gen.go")
	want(t, run(t, "", "dbc", "generate", "testdata/vehicle.dbc", "--package", "veh", "-o", gen), 0)
	for _, p := range []string{trace, gen} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if m := st.Mode().Perm(); m != 0o600 {
			t.Errorf("%s: mode %o, want 600", filepath.Base(p), m)
		}
	}
}

func TestTwosComplement(t *testing.T) {
	for v, w := range map[int64]uint64{0: 0, 1: 1, -1: math.MaxUint64, math.MinInt64: 1 << 63, math.MaxInt64: math.MaxInt64, -2: math.MaxUint64 - 1} {
		if got := twosComplement(v); got != w {
			t.Errorf("twosComplement(%d) = %#x, want %#x", v, got, w)
		}
	}
}

// SPDX-License-Identifier: MIT

package cmd

import (
	"math"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// wantWide is want for a command given an int flag value beyond 32 bits:
// on a 32-bit platform the flag refuses it, a usage error (exit 2).
func wantWide(t *testing.T, r result, code int, contains ...string) {
	t.Helper()
	if strconv.IntSize == 32 {
		want(t, r, 2, "invalid argument", "value out of range")
		return
	}
	want(t, r, code, contains...)
}

// TestIntFlagsAreIntFlag: every int flag of every command is an intFlag
// (intVar), which parses at the platform's int size. pflag's own int flag
// parses 64 bits and kept the low 32 on 32-bit platforms.
func TestIntFlagsAreIntFlag(t *testing.T) {
	n := 0
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		check := func(f *pflag.Flag) {
			if f.Value.Type() != "int" {
				return
			}
			n++
			if _, ok := f.Value.(intFlag); !ok {
				t.Errorf("%s --%s: %T, not intFlag (intVar)", c.CommandPath(), f.Name, f.Value)
			}
		}
		c.LocalNonPersistentFlags().VisitAll(check)
		c.PersistentFlags().VisitAll(check)
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(newRoot(&app{}))
	if n < 20 {
		t.Fatalf("%d int flags found: the walk missed some", n)
	}
}

// TestIntFlagBounds: the boundary table of intFlag. A value an int holds
// on this platform is taken as it is; one beyond it is refused, as pflag
// refuses one beyond 64 bits.
func TestIntFlagBounds(t *testing.T) {
	maxInt, minInt := strconv.Itoa(math.MaxInt), strconv.Itoa(math.MinInt)
	wide := strconv.IntSize == 64
	for _, c := range []struct {
		in   string
		ok   bool
		want int64
	}{
		{"0", true, 0},
		{"1", true, 1},
		{"-1", true, -1},
		{"0x10", true, 16},
		{"2147483646", true, 2147483646},
		{"2147483647", true, 2147483647},
		{"-2147483648", true, -2147483648},
		{"2147483648", wide, 2147483648},
		{"-2147483649", wide, -2147483649},
		{"4294967296", wide, 4294967296},
		{"4294967297", wide, 4294967297},
		{"9223372036854775806", wide, 9223372036854775806},
		{"9223372036854775807", wide, 9223372036854775807},
		{"-9223372036854775808", wide, -9223372036854775808},
		{"9223372036854775808", false, 0},
		{"-9223372036854775809", false, 0},
		{"18446744073709551617", false, 0},
		{maxInt, true, math.MaxInt},
		{minInt, true, math.MinInt},
		{"", false, 0},
		{"1.5", false, 0},
		{"ten", false, 0},
	} {
		v := 7
		f := intFlag{&v}
		err := f.Set(c.in)
		if (err == nil) != c.ok {
			t.Fatalf("Set(%q): %v, want ok %v", c.in, err, c.ok)
		}
		if !c.ok {
			if v != 7 {
				t.Fatalf("Set(%q) refused but stored %d", c.in, v)
			}
			continue
		}
		if int64(v) != c.want || f.String() != strconv.FormatInt(c.want, 10) {
			t.Fatalf("Set(%q): %d (%s), want %d", c.in, v, f.String(), c.want)
		}
	}
}

// TestIntFlagValuesBeyond32Bits: through the commands, a value beyond 32
// bits is a usage error on a 32-bit platform. It was taken modulo 2^32:
// --repeat 4294967297 sent one frame, monitor -n 4294967296 had no limit.
func TestIntFlagValuesBeyond32Bits(t *testing.T) {
	for _, v := range []string{"2147483648", "4294967297", "9223372036854775807"} {
		wantWide(t, run(t, "", "send", "-i", "virtual", "--repeat", v, "123#00"), 3, "exceed --max-frames 10000")
		wantWide(t, run(t, "", "xl", "send", "-i", "virtual", "--repeat", v, "123#80:00:00000000#00"), 3, "exceed --max-frames")
		wantWide(t, run(t, "", "send", "-i", "virtual", "--max-frames", v, "123#00"), 0, "sent 1 frame(s)")
		wantWide(t, run(t, "", "monitor", "-i", "virtual", "-n", v, "--duration", "1ms"), 0)
		wantWide(t, run(t, "", "trace", "info", "testdata/trace.log", "--slcan-baud", v), 0, "4 frames")
	}
	// 2^31-1 fits everywhere.
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "2147483647", "123#00"), 3, "exceed --max-frames 10000")
	want(t, run(t, "", "monitor", "-i", "virtual", "-n", "2147483647", "--duration", "1ms"), 0)
	// Beyond 64 bits is a usage error everywhere, as before.
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "9223372036854775808", "123#00"), 2, "invalid argument")
	want(t, run(t, "", "monitor", "-i", "virtual", "-n", "-9223372036854775809", "--duration", "1ms"), 2, "invalid argument")
}

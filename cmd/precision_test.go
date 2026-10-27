// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"
)

// TestHumanFloats: text output rounds float noise away (0.1 × 3 is
// 0.30000000000000004 in float64), JSON keeps the full value and the
// exact raw string.
func TestHumanFloats(t *testing.T) {
	r := run(t, "", "dbc", "decode", "testdata/precision.dbc", "002#0300")
	want(t, r, 0, "Throttle=0.3 %")
	if strings.Contains(r.out, "0.30000000000000004") {
		t.Fatal(r.out)
	}
	r = run(t, "", "dbc", "decode", "testdata/precision.dbc", "002#0300", "--format", "json")
	want(t, r, 0, `"physical":0.30000000000000004`, `"raw":"3"`)
	for in, want := range map[float64]string{0.1 * 3: "0.3", 582.5: "582.5", 1e21: "1e+21", -40: "-40", 18446744073709551615: "1.84467440737e+19"} {
		if got := human(in); got != want {
			t.Errorf("human(%v) = %q, want %q", in, got, want)
		}
	}
	want(t, run(t, "", "dbc", "inspect", "testdata/precision.dbc", "-m", "Pedal"), 0, "×0.1 +0 %")
}

// TestEncodeExact64: a 64-bit unsigned signal with factor 1 encodes the
// decimal argument exactly, without passing through float64.
func TestEncodeExact64(t *testing.T) {
	r := run(t, "", "dbc", "encode", "testdata/precision.dbc", "Wide64", "U64=18446744073709551615")
	want(t, r, 0, "001#FFFFFFFFFFFFFFFF")
	r = run(t, "", "dbc", "encode", "testdata/precision.dbc", "Wide64", "U64=18446744073709551614")
	want(t, r, 0, "001#FEFFFFFFFFFFFFFF")
	r = run(t, "", "dbc", "decode", "testdata/precision.dbc", "001#FEFFFFFFFFFFFFFF", "--format", "json")
	want(t, r, 0, `"raw":"18446744073709551614"`)
	want(t, run(t, "", "dbc", "encode", "testdata/precision.dbc", "Pedal", "Throttle=0.3"), 0, "002#03")
}

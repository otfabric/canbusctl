// SPDX-License-Identifier: MIT

package cmd

import (
	"testing"
)

func TestSimulate(t *testing.T) {
	r := run(t, "", "simulate", "--dbc", vehicle, "--duration", "120ms", "--default-cycle", "40ms")
	want(t, r, 0, "virtual 100#", "Engine: Speed=", "Status: Page=0", "simulated")
	// Deterministic: the same seed gives the same frames (timestamps aside).
	a := run(t, "", "simulate", "--dbc", vehicle, "--pattern", "random", "--seed", "7", "--max-frames", "8", "--format", "json", "--message", "Engine", "--message", "Status")
	b := run(t, "", "simulate", "--dbc", vehicle, "--pattern", "random", "--seed", "7", "--max-frames", "8", "--format", "json", "--message", "Engine", "--message", "Status")
	ra, rb := jsonLines(t, a.out), jsonLines(t, b.out)
	if a.code != 0 || len(ra) != 9 || len(rb) != 9 {
		t.Fatal(a, b)
	}
	for i := 0; i < 8; i++ {
		if ra[i]["data"] != rb[i]["data"] || ra[i]["idHex"] != rb[i]["idHex"] {
			t.Fatalf("frame %d differs: %v %v", i, ra[i], rb[i])
		}
	}
	if ra[8]["type"] != "summary" || ra[8]["sent"].(float64) != 8 {
		t.Fatal(ra[8])
	}
	want(t, run(t, "", "simulate", "--dbc", vehicle, "--pattern", "initial", "--max-frames", "3", "--format", "table"), 0, "simulated 3")
	want(t, run(t, "", "simulate", "--dbc", "testdata/sample.kcd", "--max-frames", "2", "--default-cycle", "5ms"), 0, "simulated 2")
	want(t, run(t, "", "simulate", "--dbc", vehicle, "--pattern", "zig"), 3, "--pattern")
	want(t, run(t, "", "simulate", "--dbc", vehicle, "--duration", "0"), 3, "bounded")
	want(t, run(t, "", "simulate", "--dbc", vehicle, "--message", "Wide"), 3, "--default-cycle")
	want(t, run(t, "", "simulate", "--dbc", "testdata/missing.dbc"), 3)
	want(t, run(t, "", "simulate"), 2, "dbc")
	r = run(t, "", "simulate", "--dbc", vehicle, "-i", "can0", "--max-frames", "1")
	if linux() {
		want(t, r, 4)
	} else {
		want(t, r, 8)
	}
}

func TestInject(t *testing.T) {
	r := run(t, "", "inject", "--frame", "123#01", "--frame", "456#02", "--duplicate", "1", "--error-frame", "bus-off", "--malformed", "2")
	want(t, r, 0, "virtual 123#01", "(error frame)", "MALFORMED", "2 duplicated", "error frame bus-off", "2 malformed")
	r = run(t, "", "inject", "--trace", "testdata/trace.log", "--drop", "1", "--format", "json")
	want(t, r, 0, `"dropped": 3`, `"delivered": 0`)
	r = run(t, "", "inject", "--frame", "123#0102", "--corrupt", "1", "--delay-probability", "1", "--delay", "5ms", "--seed", "3")
	want(t, r, 0, "1 delayed, 1 corrupted")
	want(t, run(t, "", "inject"), 3, "nothing to inject")
	want(t, run(t, "", "inject", "--frame", "1#"), 3)
	want(t, run(t, "", "inject", "--drop", "2", "--frame", "123#"), 3, "between 0 and 1")
	want(t, run(t, "", "inject", "--error-frame", "lava"), 3, "--error-frame")
	want(t, run(t, "", "inject", "-i", "vcan0", "--error-frame", "bus-off"), 8, "virtual bus")
	want(t, run(t, "", "inject", "--trace", "testdata/trace.log", "--max-frames", "1"), 3, "--max-frames")
	want(t, run(t, "", "inject", "--trace", "testdata/missing.log"), 3)
	r = run(t, "", "inject", "-i", "can0", "--frame", "123#")
	if linux() {
		want(t, r, 4)
	} else {
		want(t, r, 8)
	}
}

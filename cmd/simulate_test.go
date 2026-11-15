// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strings"
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
	for i := range 8 {
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
	want(t, r, 0, "1 delayed, 0 reordered, 1 corrupted")
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

func TestDecodeNaNAsJSON(t *testing.T) {
	r := run(t, "", "dbc", "decode", "testdata/floats.dbc", "001##00000C07F000000000000F87F00000000", "--format", "json")
	want(t, r, 0, `"physical":"NaN"`)
}

// TestSimulateCycleTimeRange: a cycle time from the database outside
// 1..3600000 ms (1 hour) is ignored with a warning, and the message is
// treated as one without a cycle time: simulated every --default-cycle,
// or skipped without it. The range is checked on the database's 64-bit
// value, before the period is computed, on every platform. Every value
// was taken without a warning.
func TestSimulateCycleTimeRange(t *testing.T) {
	const head = "VERSION \"\"\n\nNS_ :\n\nBS_:\n\nBU_: ECU\n\nBO_ 256 M: 1 ECU\n SG_ S : 0|8@1+ (1,0) [0|255] \"\" ECU\n\n" +
		"BA_DEF_ BO_ \"GenMsgCycleTime\" INT 0 0;\n"
	const kcd = `<?xml version="1.0" encoding="UTF-8"?>
<NetworkDefinition xmlns="http://kayak.2codeornot2code.org/1.0">
  <Document name="t" version="1"/>
  <Node id="1" name="A"/>
  <Bus name="Main">
    <Message id="0x10" name="M" length="1" interval="%s">
      <Producer><NodeRef id="1"/></Producer>
      <Signal name="S" offset="0" length="8"/>
    </Message>
  </Bus>
</NetworkDefinition>
`
	type cycleCase struct {
		name, db string
		ok       bool // in range: simulated at it, without a warning
	}
	var cases []cycleCase
	for _, v := range []string{"1", "3600000", "3600001", "4294967297", "9223372036854", "9223372036855", "9223372036854775807", "-1", "-9223372036854775808"} {
		ok := v == "1" || v == "3600000"
		cases = append(cases,
			cycleCase{"BA_ " + v, writeFile(t, "ba.dbc", head+"BA_ \"GenMsgCycleTime\" BO_ 256 "+v+";\n"), ok},
			cycleCase{"BA_DEF_DEF_ " + v, writeFile(t, "def.dbc", head+"BA_DEF_DEF_ \"GenMsgCycleTime\" "+v+";\n"), ok})
	}
	for _, v := range []string{"1", "3600000", "3600001", "-5"} {
		cases = append(cases, cycleCase{"KCD interval " + v, writeFile(t, "ct.kcd", fmt.Sprintf(kcd, v)), v == "1" || v == "3600000"})
	}
	for _, c := range cases {
		r := run(t, "", "simulate", "--dbc", c.db, "--duration", "100ms", "--max-frames", "2", "--format", "json")
		warned := strings.Contains(r.errOut, "is outside 1..3600000 ms: ignored")
		if c.ok {
			want(t, r, 0)
			if warned {
				t.Fatalf("%s: warned: %s", c.name, r.errOut)
			}
			continue
		}
		want(t, r, 3, "no message to simulate", "skipped (no --default-cycle)")
		if !warned {
			t.Fatalf("%s: no warning: %s", c.name, r.errOut)
		}
		r = run(t, "", "simulate", "--dbc", c.db, "--duration", "1s", "--max-frames", "2", "--default-cycle", "5ms", "--format", "json")
		want(t, r, 0, "simulated every --default-cycle 5ms")
		recs := jsonLines(t, r.out)
		if s := recs[len(recs)-1]; s["type"] != "summary" || s["sent"] != float64(2) {
			t.Fatalf("%s: %v", c.name, s)
		}
	}
	// 1 hour is a period, not "no cycle time": one frame in --duration.
	r := run(t, "", "simulate", "--dbc", writeFile(t, "h.dbc", head+"BA_ \"GenMsgCycleTime\" BO_ 256 3600000;\n"), "--duration", "100ms", "--format", "json")
	recs := jsonLines(t, r.out)
	if s := recs[len(recs)-1]; r.code != 0 || s["sent"] != float64(1) {
		t.Fatalf("exit %d: %v %s", r.code, s, r.errOut)
	}
	// 0 is "no cycle time", as before: no warning.
	r = run(t, "", "simulate", "--dbc", writeFile(t, "z.dbc", head+"BA_ \"GenMsgCycleTime\" BO_ 256 0;\n"))
	want(t, r, 3, "no message to simulate")
	if strings.Contains(r.errOut, "outside") {
		t.Fatal(r.errOut)
	}
}

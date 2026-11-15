// SPDX-License-Identifier: MIT

package cmd

import (
	"strings"
	"testing"
)

// terminalSafe fails when out holds a character a terminal acts on: a C0
// control other than line feed and tab, DEL, a C1 control or a Unicode
// bidirectional formatting character.
func terminalSafe(t *testing.T, what, out string) {
	t.Helper()
	for _, r := range out {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r >= 0x80 && r <= 0x9f ||
			r == 0x061c || r == 0x200e || r == 0x200f || r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069 {
			t.Fatalf("%s: %U written:\n%q", what, r, out)
		}
	}
}

// TestDatabaseAndTraceStringsEscaped: the strings of a database (units,
// value labels, comments) and of a trace (interface names) are escaped in
// text and table output, and kept as they are in JSON. They reached the
// terminal as they came.
func TestDatabaseAndTraceStringsEscaped(t *testing.T) {
	const esc = "\x1b]0;x\x07"
	const bidi = "\U0000202e"
	db := writeFile(t, "ctl.dbc", "VERSION \"\"\n\nNS_ :\n\nBS_:\n\nBU_: ECU\n\n"+
		"BO_ 256 M: 2 ECU\n SG_ S : 0|8@1+ (1,0) [0|255] \"u"+esc+"\" ECU\n SG_ L : 8|8@1+ (1,0) [0|255] \"\" ECU\n\n"+
		"CM_ BO_ 256 \"c"+esc+"\";\n"+
		"VAL_ 256 L 1 \"one"+bidi+"\" ;\n")
	trace := writeFile(t, "ctl.jsonl", "{\"type\":\"header\",\"version\":1}\n"+
		"{\"type\":\"frame\",\"timestamp\":\"1700000000.000000000\",\"interface\":\"can\\u001b[2J0\\u202e\",\"id\":256,\"len\":2,\"data\":\"0101\"}\n")
	for _, c := range []struct {
		args []string
		want []string // escaped forms in the text output
	}{
		{[]string{"dbc", "decode", db, "--trace", trace}, []string{`can\x1b[2J0`, `S=1 u\x1b]0;x\x07`, `(one\` + "u202e)"}},
		{[]string{"dbc", "decode", db, "100#0101"}, []string{`u\x1b]0;x\x07`}},
		{[]string{"dbc", "inspect", db, "--message", "M"}, []string{`u\x1b]0;x\x07`}},
		{[]string{"dbc", "inspect", db}, []string{"M"}},
		{[]string{"dbc", "inspect", db, "--format", "table"}, []string{"M"}},
		{[]string{"trace", "info", trace, "--format", "table"}, []string{"100"}},
	} {
		r := run(t, "", c.args...)
		want(t, r, 0, c.want...)
		terminalSafe(t, strings.Join(c.args, " "), r.out)
	}
	// JSON keeps the strings (JSON escapes the control characters).
	r := run(t, "", "dbc", "decode", db, "--trace", trace, "--format", "json")
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) == 0 || recs[0]["interface"] != "can\x1b[2J0"+bidi {
		t.Fatalf("JSON interface changed: %v", recs)
	}
}

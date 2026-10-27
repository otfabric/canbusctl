// SPDX-License-Identifier: MIT

package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
)

func TestParseID(t *testing.T) {
	if id, err := ParseID("0x7FF", false); err != nil || id != 0x7FF {
		t.Fatal(id, err)
	}
	if id, err := ParseID("18DAF110", true); err != nil || id != 0x18DAF110 {
		t.Fatal(id, err)
	}
	for _, bad := range []string{"800", "zz", ""} {
		if _, err := ParseID(bad, false); err == nil {
			t.Error(bad)
		}
	}
	if _, err := ParseID("20000000", true); err == nil {
		t.Error("30 bits")
	}
	if b, err := ParseHexByte("0xCC"); err != nil || b != 0xCC {
		t.Fatal(b, err)
	}
}

func TestParseAssignments(t *testing.T) {
	in, err := ParseAssignments([]string{"A=1.5", " B = Second ", `C="On"`}, false)
	if err != nil || len(in) != 3 {
		t.Fatal(in, err)
	}
	if in["A"] != candb.PhysText("1.5") || in["B"] != candb.Label("Second") || in["C"] != candb.Label("On") {
		t.Fatal(in)
	}
	raw, err := ParseAssignments([]string{"A=0x10", "B=-3", "C=18446744073709551615"}, true)
	if err != nil || raw["A"] != candb.Raw(16) || raw["B"] != candb.RawInt(-3) || raw["C"] != candb.Raw(^uint64(0)) {
		t.Fatal(raw, err)
	}
	for _, bad := range [][]string{{"A"}, {"=1"}, {"A="}, {"A=1", "A=2"}} {
		if _, err := ParseAssignments(bad, false); err == nil {
			t.Error(bad)
		}
	}
	if _, err := ParseAssignments([]string{"A=1.5"}, true); err == nil {
		t.Error("raw float")
	}
}

func TestFiltersAndFrames(t *testing.T) {
	fs, err := ParseFilters([]string{"123:7FF", "12345678~1FFFFFFF"}, true, true)
	if err != nil || len(fs.Filters) != 2 || !fs.Join || fs.ErrorMask != canbus.ErrClassAll {
		t.Fatal(fs, err)
	}
	if _, err := ParseFilters([]string{"x"}, false, false); err == nil {
		t.Fatal("bad filter")
	}
	for _, bad := range []string{"123#GG", "20000040#00", "800#"} {
		if _, err := ParseFrame(bad); err == nil {
			t.Error(bad)
		}
	}
	f, _ := ParseFrame("1ABCDEF0##1AA")
	j := ToJSON(&f)
	if j.IDHex != "1ABCDEF0" || !j.FD || !j.BRS || j.Data != "aa" || j.Timestamp != "" {
		t.Fatal(j)
	}
	f.Timestamp = time.Unix(5, 1500)
	if FrameLine(&f) != "(5.000001) - 1ABCDEF0##1AA" {
		t.Fatal(FrameLine(&f))
	}
	ef := canbus.NewErrorFrame(canbus.ErrorInfo{Class: canbus.ErrClassBusOff})
	if !strings.Contains(FrameLine(&ef), "ERROR bus-off") || ToJSON(&ef).IDHex != "00000040" {
		t.Fatal(FrameLine(&ef))
	}
}

func TestTraceFormat(t *testing.T) {
	for p, want := range map[string]string{"a.log": "candump", "a.JSONL": "jsonl", "a.pcap": "pcap", "a": "candump"} {
		if f, err := TraceFormat(p, ""); err != nil || string(f) != want {
			t.Error(p, f, err)
		}
	}
	if f, _ := TraceFormat("a.log", "pcap"); f != "pcap" {
		t.Fatal(f)
	}
	if _, err := TraceFormat("a", "asc"); err == nil {
		t.Fatal("asc")
	}
	lines, _ := ReadLines(strings.NewReader("a\n\n# c\n b \n"))
	if len(lines) != 2 || lines[1] != "b" {
		t.Fatal(lines)
	}
	if TransmitCheck(Virtual, false, "x") != nil || TransmitCheck("can9", true, "x") != nil {
		t.Fatal("TransmitCheck shortcuts")
	}
}

func FuzzParseFrame(f *testing.F) {
	for _, s := range []string{"123#DEADBEEF", "12345678##3AABB", "123#R5", "123#1122334455667788_C"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fr, err := ParseFrame(s)
		if err != nil {
			return
		}
		if fr.Validate() != nil || fr.IsError() {
			t.Fatalf("%q accepted as %v", s, fr)
		}
		_ = ToJSON(&fr)
	})
}

func FuzzParseAssignments(f *testing.F) {
	f.Add("Speed=1.5", true)
	f.Fuzz(func(t *testing.T, s string, raw bool) {
		in, err := ParseAssignments(strings.Split(s, ","), raw)
		if err == nil && len(in) == 0 && s != "" {
			t.Fatal("no assignment and no error")
		}
	})
}

func FuzzParseFilters(f *testing.F) {
	f.Add("123:7FF,std")
	f.Fuzz(func(t *testing.T, s string) {
		fs, err := ParseFilters([]string{s}, false, false)
		if err == nil && len(fs.Filters) != 1 {
			t.Fatal("filter lost")
		}
	})
}

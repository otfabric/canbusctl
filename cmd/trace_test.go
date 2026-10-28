// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
)

// traceFrames returns the frames of a trace file.
func traceFrames(t *testing.T, p string) []canbus.Frame {
	t.Helper()
	r, closer, err := openTraceForTest(p)
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	var out []canbus.Frame
	for {
		var f canbus.Frame
		if r.Read(&f) != nil {
			return out
		}
		out = append(out, f)
	}
}

func TestTraceConvertChain(t *testing.T) {
	prev := time.Local
	time.Local = time.UTC // ASC and BLF hold zone-less local times
	defer func() { time.Local = prev }()
	src := writeTrace(t, capture.FormatCandump, vehicleFrames(t))
	dir := t.TempDir()
	in := src
	for _, ext := range []string{".jsonl", ".pcap", ".asc", ".blf", ".trc", ".log"} {
		out := filepath.Join(dir, "c"+ext)
		r := run(t, "", "trace", "convert", in, out)
		want(t, r, 0, "3 frames read, 3 written")
		in = out
	}
	got, orig := traceFrames(t, in), traceFrames(t, src)
	if len(got) != len(orig) {
		t.Fatal(len(got))
	}
	for i := range got {
		g, o := got[i], orig[i]
		if g.ID != o.ID || g.Flags&^canbus.FlagBRS != o.Flags&^canbus.FlagBRS || string(g.Payload()) != string(o.Payload()) || !g.Timestamp.Equal(o.Timestamp) {
			t.Fatalf("frame %d: %v != %v", i, g, o)
		}
	}
	want(t, run(t, "", "dbc", "decode", vehicle, "--trace", filepath.Join(dir, "c.blf")), 0, "Engine", "Speed=582.5")
	want(t, run(t, "", "dbc", "decode", vehicle, "--trace", filepath.Join(dir, "c.asc")), 0, "Speed=582.5")
}

func TestTraceConvertOptions(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "f.log")
	r := run(t, "", "trace", "convert", "testdata/trace.log", out, "--filter", "100:7FF", "--format", "json")
	want(t, r, 0)
	// Filters select data frames; error frames pass unless --no-errors.
	doc := jsonDoc(t, r.out)["result"].(map[string]any)
	if doc["read"].(float64) != 4 || doc["written"].(float64) != 2 || doc["filtered"].(float64) != 2 {
		t.Fatal(doc)
	}
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", out, "--max-frames", "2"), 0, "2 written")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", out, "--filter", "100:700", "--filter", "100:0FF", "--join", "--no-errors"), 0, "1 written")
	// Interface names and error classes are lost in ASC/BLF; the loss is reported.
	errLog := filepath.Join(dir, "e.log")
	_ = os.WriteFile(errLog, []byte("(1700000000.000000) can0 20000040#0000000000000000\n(1700000000.000001) can0 123#01\n"), 0o644)
	r = run(t, "", "trace", "convert", errLog, filepath.Join(dir, "e.asc"))
	want(t, r, 0, "lost: 1 frames: error class", "lost: 2 frames: interface name")
	want(t, run(t, "", "trace", "convert", errLog, filepath.Join(dir, "e.blf"), "--no-errors"), 0, "1 written", "1 filtered out")
	want(t, run(t, "", "trace", "convert", errLog, filepath.Join(dir, "e.pcap")), 0, "lost: 2 frames: interface name")
	// stdin input and an explicit format.
	want(t, run(t, "", "trace", "convert", "-", filepath.Join(dir, "s.out"), "--file-format", "jsonl"), 3, "unknown trace format")
	in, _ := os.ReadFile("testdata/trace.log")
	want(t, run(t, string(in), "trace", "convert", "-", filepath.Join(dir, "s.out"), "--file-format", "jsonl"), 0, "4 written")
	// Errors.
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", "x.pcapng"), 8, "pcapng")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", out, "--file-format", "mf4"), 3, "unknown trace format")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", out, "--filter", "zz"), 3)
	want(t, run(t, "", "trace", "convert", "testdata/missing.log", out), 3)
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", filepath.Join(dir, "no/x.log")), 3, "cannot create")
	bad := filepath.Join(dir, "bad.log")
	_ = os.WriteFile(bad, []byte("(1700000000.000000) can0 123#01\nnot a frame\n"), 0o644)
	want(t, run(t, "", "trace", "convert", bad, out), 3)
	want(t, run(t, "", "trace", "convert", "testdata/trace.log"), 2)
	if (&losses{echo: 1, subMicro: 2}).list()[1] != "2 frames: timestamp below microseconds" {
		t.Fatal("losses")
	}
	var l losses
	f := canbus.Frame{Echo: true, Timestamp: time.Unix(0, 1)}
	l.note(&f, capture.FormatCandump)
	l.note(&f, capture.FormatPCAP)
	l.note(&f, capture.FormatJSONL)
	if l.echo != 2 || l.subMicro != 1 {
		t.Fatal(l)
	}
}

func TestTraceInfo(t *testing.T) {
	r := run(t, "", "trace", "info", "testdata/trace.log")
	want(t, r, 0, "testdata/trace.log (candump): 4 frames over 0.030000 s", "classical 3", "error 1", "18FEF1FE", "7FF")
	r = run(t, "", "trace", "info", "testdata/trace.log", "--format", "json")
	doc := jsonDoc(t, r.out)["result"].(map[string]any)
	if doc["frames"].(float64) != 4 || len(doc["ids"].([]any)) != 4 {
		t.Fatal(doc)
	}
	r = run(t, "", "trace", "info", "testdata/trace.log", "--format", "table")
	want(t, r, 0, "RATE_HZ", "33.33")
	in, _ := os.ReadFile("testdata/trace.log")
	want(t, run(t, string(in), "trace", "info", "-"), 0, "4 frames")
	fd := writeTrace(t, capture.FormatJSONL, append(vehicleFrames(t), canbus.Frame{ID: 0x321, Flags: canbus.FlagRemote, Len: 2}))
	want(t, run(t, "", "trace", "info", fd), 0, "CAN FD 1, remote 1")
	want(t, run(t, "", "trace", "info", "testdata/missing.log"), 3)
	want(t, run(t, "", "trace", "info", "testdata/vehicle.dbc"), 3)
	bad := filepath.Join(t.TempDir(), "bad.log")
	_ = os.WriteFile(bad, []byte("(1700000000.000000) can0 123#01\nnot a frame\n"), 0o644)
	want(t, run(t, "", "trace", "info", bad), 3)
	empty := filepath.Join(t.TempDir(), "e.jsonl")
	_ = os.WriteFile(empty, nil, 0o644)
	r = run(t, "", "trace", "info", empty)
	if r.code == 0 && !strings.Contains(r.out, "0 frames") {
		t.Fatal(r.out)
	}
}

func TestMonitorOutput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "m.blf")
	r := runWithTraffic(t, vehicleFrames(t), "monitor", "-i", "virtual", "-n", "3", "-o", out)
	want(t, r, 0, "3 frames")
	if n := len(traceFrames(t, out)); n != 3 {
		t.Fatal(n)
	}
	want(t, run(t, "", "monitor", "-i", "virtual", "-o", "x.pcapng"), 8, "pcapng")
	want(t, run(t, "", "monitor", "-i", "virtual", "-o", filepath.Join(t.TempDir(), "no/x.log")), 3, "cannot create")
}

func TestDatabasesV2(t *testing.T) {
	arx := "testdata/system.arxml"
	want(t, run(t, "", "dbc", "inspect", arx), 0, "Engine")
	want(t, run(t, "", "dbc", "validate", arx), 0)
	r := run(t, "", "dbc", "convert", arx, "--to", "dbc")
	want(t, r, 0, "BO_ 256 Engine")
	dir := t.TempDir()
	kcdOut := filepath.Join(dir, "v.kcd")
	r = run(t, "", "dbc", "convert", vehicle, "--to", "kcd", "-o", kcdOut)
	want(t, r, 0)
	want(t, run(t, "", "dbc", "inspect", kcdOut), 0, "Engine")
	want(t, run(t, "", "dbc", "convert", arx, "--to", "kcd"), 0, "<NetworkDefinition")
	want(t, run(t, "", "dbc", "convert", "testdata/sample.kcd", "--to", "json"), 0, `"messages"`)
	// An extension-less ARXML is recognised by its content.
	noext := filepath.Join(dir, "system")
	b, _ := os.ReadFile(arx)
	_ = os.WriteFile(noext, b, 0o644)
	want(t, run(t, "", "dbc", "inspect", noext), 0, "Engine")
	broken := filepath.Join(dir, "x.arxml")
	_ = os.WriteFile(broken, []byte("<AUTOSAR><AR-PACKAGES>"), 0o644)
	want(t, run(t, "", "dbc", "inspect", broken), 3)
}

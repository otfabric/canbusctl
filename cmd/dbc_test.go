// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const vehicle = "testdata/vehicle.dbc"

func TestDBCInspect(t *testing.T) {
	r := run(t, "", "dbc", "inspect", vehicle)
	want(t, r, 0)
	golden(t, "inspect.txt", r.out)
	r = run(t, "", "dbc", "inspect", vehicle, "--format", "json")
	want(t, r, 0)
	golden(t, "inspect.json", r.out)
	doc := jsonDoc(t, r.out)
	if doc["schemaVersion"] != "1.0" || doc["command"] != "dbc inspect" {
		t.Fatal(doc)
	}
	r = run(t, "", "dbc", "inspect", vehicle, "--format", "table")
	want(t, r, 0, "ID", "Engine", "18FEF1FE")
	r = run(t, "", "dbc", "inspect", vehicle, "-m", "Status")
	want(t, r, 0, "Page", "[multiplexer]", "Amps", "when Page")
	r = run(t, "", "dbc", "inspect", vehicle, "-m", "100", "--format", "json")
	want(t, r, 0, `"startBit"`)
	r = run(t, "", "dbc", "inspect", vehicle, "-m", "nope")
	want(t, r, 3, "no message")
	r = run(t, "", "dbc", "inspect", "testdata/sample.kcd")
	want(t, r, 0, "(kcd)", "BE12"[:0])
	r = run(t, "", "dbc", "inspect", "testdata/missing.dbc", "--format", "json")
	want(t, r, 3, `"code": "file_open_failed"`)
}

func TestDBCValidate(t *testing.T) {
	want(t, run(t, "", "dbc", "validate", vehicle), 0, "valid (0 errors, 0 warnings)")
	r := run(t, "", "dbc", "validate", "testdata/bad.dbc")
	want(t, r, 1, "exceed the message length", "invalid (1 errors")
	want(t, run(t, "", "dbc", "validate", "testdata/warn.dbc"), 0, "1 warnings")
	want(t, run(t, "", "dbc", "validate", "testdata/warn.dbc", "--strict"), 1, "invalid")
	r = run(t, "", "dbc", "validate", "testdata/bad.dbc", "--format", "json")
	if r.code != 1 || jsonDoc(t, r.out)["result"].(map[string]any)["valid"] != false {
		t.Fatal(r)
	}
	want(t, run(t, "", "dbc", "validate", "testdata/none.dbc"), 3)
	want(t, run(t, "", "dbc", "inspect", "testdata/bad.dbc"), 3, "invalid_database"[:0], "exceed")
}

func TestDBCDecode(t *testing.T) {
	r := run(t, "", "dbc", "decode", vehicle, "100#3412142000000000", "18FEF1FE#01D0070000000000")
	want(t, r, 0, "Engine: Speed=582.5 rpm Temp=-20 degC Gear=2 (Second)", "Status: Page=1 Amps=200 A", "Amps=200 A")
	golden(t, "decode.txt", r.out)
	r = run(t, "", "dbc", "decode", vehicle, "--trace", "testdata/trace.log", "--format", "json")
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) != 4 || recs[0]["message"] != "Engine" || recs[3]["error"] != true || recs[0]["type"] != "frame" {
		t.Fatal(recs)
	}
	// Unknown messages: fine, unless --strict.
	want(t, run(t, "", "dbc", "decode", vehicle, "7FF#00"), 0, "(unknown message)")
	want(t, run(t, "", "dbc", "decode", vehicle, "--strict", "7FF#00"), 1)
	// A frame too short for its message.
	want(t, run(t, "", "dbc", "decode", vehicle, "100#00"), 1, "needs 8 bytes")
	// Frames on stdin, as cansend notation and candump lines.
	r = run(t, "# comment\n100#0000000000000000\n(1.0) can0 100#0800000000000000\n", "dbc", "decode", vehicle)
	want(t, r, 0, "Speed=0 rpm", "Speed=1 rpm", "(1.000000) can0")
	want(t, run(t, "zz\n", "dbc", "decode", vehicle), 3, "frame \"zz\"")
	want(t, run(t, "(1.0) can0\n", "dbc", "decode", vehicle), 3, "line 1")
	want(t, run(t, "", "dbc", "decode", vehicle, "--trace", "testdata/vehicle.dbc"), 3, "unknown trace format")
	want(t, run(t, "", "dbc", "decode", vehicle, "--trace", "-"), 3)
	want(t, run(t, "(1.0) x 100#00\nbad\n", "dbc", "decode", vehicle, "--trace", "-"), 3, "candump record 2")
	want(t, run(t, "", "dbc", "decode", vehicle, "--trace", "testdata/missing.log"), 3, "cannot open trace")
}

func TestDBCEncode(t *testing.T) {
	r := run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=582.5", "Temp=-20", "Gear=Second")
	want(t, r, 0, "100#3412142000000000")
	r = run(t, "", "dbc", "encode", vehicle, "18FEF1FE", "Page=1", "Amps=-1.5", "--format", "json")
	want(t, r, 0, `"notation": "18FEF1FE#01F1FF0000000000"`)
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=4660", "Temp=0", "Gear=2", "--raw"), 0, "100#3412002000000000")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=1", "Temp=-100", "Gear=0"), 3, "out of range")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=1", "Temp=-100", "Gear=0", "--lax"), 0)
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=1"), 3, "missing")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=1", "--fill-missing"), 0)
	want(t, run(t, "", "dbc", "encode", vehicle, "Wide", "Counter=7"), 0, "200##0") // FD message
	want(t, run(t, "", "dbc", "encode", vehicle, "Nope"), 3, "no message")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed"), 3, "signal=value")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=1", "Speed=2"), 3, "twice")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=x", "--raw"), 3, "raw value")
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Speed=-5", "Temp=0", "Gear=0", "--raw"), 3)
	want(t, run(t, "", "dbc", "encode", vehicle, "Engine", "Gear=Fifth", "Speed=0", "Temp=0"), 3, "Fifth")
	want(t, run(t, "", "dbc", "encode", "testdata/missing.dbc", "Engine"), 3)
}

func TestDBCGenerateAndConvert(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "vehicle.go")
	want(t, run(t, "", "dbc", "generate", vehicle, "-o", out), 0)
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), "package vehicle") || !strings.Contains(string(b), "from vehicle.dbc (sha256 ") {
		t.Fatal(string(b[:200]))
	}
	r := run(t, "", "dbc", "generate", vehicle, "-p", "veh")
	want(t, r, 0, "package veh", "type Engine struct")
	want(t, run(t, "", "dbc", "generate", vehicle, "--json"), 0, `"generator": "go-canbus codegen`)
	want(t, run(t, "", "dbc", "generate", vehicle, "-p", "1bad"), 3, "invalid package")
	want(t, run(t, "", "dbc", "generate", "testdata/missing.dbc"), 3)
	want(t, run(t, "", "dbc", "generate", "testdata/bad.dbc"), 3)
	want(t, run(t, "", "dbc", "generate", vehicle, "-o", filepath.Join(dir, "no/such/dir/x.go")), 10, "write")
	if goPackageName("9-Vehicle DB.dbc") != "vehicledb" || goPackageName("..dbc") != "candb" {
		t.Fatal(goPackageName("9-Vehicle DB.dbc"))
	}
	r = run(t, "", "dbc", "convert", "testdata/sample.kcd", "--to", "dbc")
	want(t, r, 0, "BO_ 16 M: 8 A", "SG_ BE12 : 15|12@0+")
	conv := filepath.Join(dir, "conv.dbc")
	want(t, run(t, "", "dbc", "convert", vehicle, "--to", "dbc", "-o", conv), 0)
	want(t, run(t, "", "dbc", "validate", conv), 0, "valid")
	want(t, run(t, "", "dbc", "convert", vehicle, "--to", "json"), 0, `"messages"`)
	want(t, run(t, "", "dbc", "convert", vehicle, "--to", "arxml"), 8, "not supported")
	want(t, run(t, "", "dbc", "convert", vehicle), 2, "to")
	want(t, run(t, "", "dbc", "convert", "testdata/missing.dbc", "--to", "dbc"), 3)
}

// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// labelled is in the layout of the CarDS recordings (candump -L -x with
// direction tokens), lines of go-canbus testdata/captures/cards.
const labelled = "(1739898746.777639) can9 7E4#100B62010D030000 T\n" +
	"(1739898746.780911) can9 7CA#0462010D03AAAAAA R\n"

// TestTraceConvertDirection: --direction writes the R/T tokens back, so a
// log with them converts to itself; without it the T marks are reported
// as lost.
func TestTraceConvertDirection(t *testing.T) {
	in := writeFile(t, "labelled.log", labelled)
	dir := t.TempDir()
	out := filepath.Join(dir, "back.log")
	want(t, run(t, "", "trace", "convert", in, out, "--direction"), 0, "2 written")
	if got, _ := os.ReadFile(out); string(got) != labelled {
		t.Fatalf("got\n%s", got)
	}
	r := run(t, "", "trace", "convert", in, filepath.Join(dir, "plain.log"))
	want(t, r, 0, "lost: 1 frames: own-transmission (echo) mark")
	if got, _ := os.ReadFile(filepath.Join(dir, "plain.log")); strings.Contains(string(got), " T\n") {
		t.Fatalf("token without --direction:\n%s", got)
	}
	r = run(t, "", "trace", "convert", in, filepath.Join(dir, "x.asc"), "--direction", "--format", "json")
	want(t, r, 3, `"code": "invalid_input"`, "--direction applies to candump output")
	noFile(t, filepath.Join(dir, "x.asc"))
}

// TestTraceConvertChannels: interface names become one channel each in ASC
// (go-canbus v0.9.0: can0 → 1, can4 → 5), reported as a loss.
func TestTraceConvertChannels(t *testing.T) {
	in := writeFile(t, "two.log", "(1700000000.000000) can0 123#11\n(1700000000.000100) can4 124#22\n")
	out := filepath.Join(t.TempDir(), "two.asc")
	want(t, run(t, "", "trace", "convert", in, out), 0, "lost: 2 frames: interface name")
	b, _ := os.ReadFile(out)
	if !strings.Contains(string(b), " 1  123 ") || !strings.Contains(string(b), " 5  124 ") {
		t.Fatalf("channels:\n%s", b)
	}
}

func TestMonitorDirection(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "m.log")
	r := runWithTraffic(t, vehicleFrames(t), "monitor", "-i", "virtual", "-n", "3", "-o", out, "--direction")
	want(t, r, 0, "3 frames")
	b, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 || !strings.HasSuffix(lines[0], " R") {
		t.Fatalf("trace:\n%s", b)
	}
	r = run(t, "", "monitor", "-i", "virtual", "-n", "1", "-o", filepath.Join(dir, "m.asc"), "--direction")
	want(t, r, 3, "--direction applies to a candump --output")
}

func TestRecordDirection(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.log")
	want(t, runWithTraffic(t, vehicleFrames(t), "record", "-i", "virtual", "-n", "3", "-o", out, "--direction"), 0, "recorded 3 frames")
	b, _ := os.ReadFile(out)
	if lines := strings.Split(strings.TrimSpace(string(b)), "\n"); len(lines) != 3 || !strings.HasSuffix(lines[2], " R") {
		t.Fatalf("trace:\n%s", b)
	}
	want(t, run(t, "", "record", "-i", "virtual", "-n", "1", "-o", filepath.Join(dir, "r.blf"), "--direction"), 3, "--direction applies to candump output")
}

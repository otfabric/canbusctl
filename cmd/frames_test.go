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

func vehicleFrames(t *testing.T) []canbus.Frame {
	a, err1 := canbus.NewStandard(0x100, []byte{0x34, 0x12, 0x14, 0x20, 0, 0, 0, 0})
	b, err2 := canbus.NewExtended(0x18FEF1FE, []byte{1, 0xD0, 7, 0, 0, 0, 0, 0})
	c, err3 := canbus.NewFD(0x200, false, true, make([]byte, 12))
	for _, err := range []error{err1, err2, err3} {
		if err != nil {
			t.Fatal(err)
		}
	}
	return []canbus.Frame{a, b, c}
}

func TestMonitor(t *testing.T) {
	r := runWithTraffic(t, vehicleFrames(t), "monitor", "-i", "virtual", "-n", "3", "--dbc", vehicle)
	want(t, r, 0, "virtual 100#3412142000000000  Engine: Speed=582.5", "200##1", "3 frames, 0 dropped")
	r = runWithTraffic(t, vehicleFrames(t), "monitor", "-i", "virtual", "-n", "2", "--filter", "18FEF1FE:1FFFFFFF", "--format", "json")
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) != 3 || recs[0]["idHex"] != "18FEF1FE" || recs[2]["type"] != "summary" || recs[2]["frames"].(float64) != 2 {
		t.Fatal(recs)
	}
	r = runWithTraffic(t, vehicleFrames(t), "monitor", "-i", "virtual", "--duration", "50ms", "--errors")
	want(t, r, 0, "frames, 0 dropped")
	want(t, run(t, "", "monitor", "-i", "virtual", "--filter", "zz"), 3, "--filter")
	want(t, run(t, "", "monitor"), 2, "interface")
	want(t, run(t, "", "monitor", "-i", "virtual", "--dbc", "testdata/missing.dbc"), 3)
	if !linux() {
		want(t, run(t, "", "monitor", "-i", "vcan0", "-n", "1"), 8, "SocketCAN is not available")
	}
}

func TestSendOnVirtual(t *testing.T) {
	r := run(t, "", "send", "-i", "virtual", "123#DEADBEEF", "1ABCDEF0#01", "7FF#R2", "--repeat", "2", "--rate", "1000")
	want(t, r, 0, "virtual 123#DEADBEEF", "virtual 7FF#R2", "sent 6 frame(s)")
	r = run(t, "", "send", "-i", "virtual", "--id", "18DA10F1", "--extended", "--data", "11.22", "--format", "json")
	want(t, r, 0, `"idHex": "18DA10F1"`, `"data": "1122"`)
	want(t, run(t, "", "send", "-i", "virtual", "--id", "7FF", "--rtr", "--dlc", "3"), 0, "7FF#R3")
	want(t, run(t, "", "send-fd", "-i", "virtual", "--id", "10", "--data", "00112233445566778899AABB", "--brs", "--esi"), 0, "010##3")
	want(t, run(t, "", "send-fd", "-i", "virtual", "123##1AABB"), 0, "sent 1")
	want(t, run(t, "", "send", "-i", "virtual", "123##1AABB"), 3, "use send-fd")
	want(t, run(t, "", "send-fd", "-i", "virtual", "123#AA"), 3, "use send")
	want(t, run(t, "", "send", "-i", "virtual"), 3, "--id and --data")
	want(t, run(t, "", "send", "-i", "virtual", "800#00"), 3, "invalid_frame"[:0], "standard identifier")
	want(t, run(t, "", "send", "-i", "virtual", "--id", "800", "--data", "00"), 3, "11-bit")
	want(t, run(t, "", "send", "-i", "virtual", "--id", "1", "--data", "0"), 3, "even number")
	want(t, run(t, "", "send", "-i", "virtual", "--id", "1", "--data", "zz"), 3, "hexadecimal")
	want(t, run(t, "", "send", "-i", "virtual", "--id", "1", "--rtr", "--dlc", "9"), 3)
	want(t, run(t, "", "send", "-i", "virtual", "20000040#00"), 3, "error frames")
	want(t, run(t, "", "send", "-i", "virtual", "001#00", "--repeat", "0"), 3, "--repeat")
	want(t, run(t, "", "send", "-i", "virtual", "123#00", "--repeat", "20", "--max-frames", "10"), 3, "max-frames")
	start := time.Now()
	want(t, run(t, "", "send", "-i", "virtual", "123#00", "--repeat", "1000", "--gap", "10ms", "--duration", "60ms"), 0, "sent")
	if time.Since(start) > 2*time.Second {
		t.Fatal("--duration did not bound send")
	}
	want(t, run(t, "", "send", "-i", "virtual", "123#00", "--repeat", "5", "--rate", "100"), 0, "sent 5")
}

func TestTransmitSafety(t *testing.T) {
	r := run(t, "", "send", "-i", "can0", "123#00")
	if linux() {
		want(t, r, 4, "can0") // no such interface
	} else {
		want(t, r, 8)
	}
	r = run(t, "", "send", "-i", "lo", "123#00", "--format", "json")
	if linux() {
		want(t, r, 9, `"code": "confirmation_required"`, "--yes")
	}
	want(t, run(t, "", "up", "vcan0"), 9, "--yes")
	want(t, run(t, "", "down", "vcan0"), 9, "--yes")
	want(t, run(t, "", "configure", "vcan0", "--bitrate", "500000"), 9, "--yes")
}

func writeTrace(t *testing.T, format capture.Format, frames []canbus.Frame) string {
	t.Helper()
	ext := map[capture.Format]string{capture.FormatCandump: ".log", capture.FormatJSONL: ".jsonl", capture.FormatPCAP: ".pcap"}[format]
	p := filepath.Join(t.TempDir(), "t"+ext)
	f, _ := os.Create(p)
	w, _ := capture.NewWriter(f, format)
	ts := time.Unix(1700000000, 0)
	for i := range frames {
		fr := frames[i]
		fr.Timestamp = ts.Add(time.Duration(i) * 5 * time.Millisecond)
		_ = w.Write(&fr)
	}
	_ = w.Close()
	_ = f.Close()
	return p
}

func TestRecordAndReplay(t *testing.T) {
	for _, ext := range []string{".log", ".jsonl", ".pcap"} {
		out := filepath.Join(t.TempDir(), "rec"+ext)
		r := runWithTraffic(t, vehicleFrames(t), "record", "-i", "virtual", "-o", out, "-n", "6")
		want(t, r, 0, "recorded 6 frames")
		rr, closer, err := openTraceForTest(out)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for {
			var f canbus.Frame
			if rr.Read(&f) != nil {
				break
			}
			n++
		}
		closer()
		if n != 6 {
			t.Fatalf("%s: %d frames in the file", ext, n)
		}
	}
	want(t, run(t, "", "record", "-i", "virtual", "-o", "x.log", "--file-format", "asc"), 3, "unknown trace format")
	want(t, run(t, "", "record", "-i", "virtual", "-o", filepath.Join(t.TempDir(), "no/x.log"), "-n", "1"), 3, "cannot create")
	want(t, runWithTraffic(t, vehicleFrames(t), "record", "-i", "virtual", "-o", filepath.Join(t.TempDir(), "d.jsonl"), "--duration", "30ms", "--file-format", "jsonl"), 0, "recorded")

	trace := writeTrace(t, capture.FormatCandump, vehicleFrames(t))
	r := run(t, "", "replay", trace, "-i", "virtual", "--print")
	want(t, r, 0, "virtual 100#3412142000000000", "replayed 3 of 3 frames")
	r = run(t, "", "replay", trace, "-i", "virtual", "--max-speed", "--filter", "100:7FF,std", "--format", "json")
	want(t, r, 0, `"sent": 1`, `"filtered": 2`)
	r = run(t, "", "replay", trace, "-i", "virtual", "--print", "--format", "json", "--max-frames", "2")
	recs := jsonLines(t, r.out)
	if r.code != 0 || len(recs) != 3 || recs[2]["limited"] != true {
		t.Fatal(r, recs)
	}
	want(t, run(t, "", "replay", "testdata/trace.log", "-i", "virtual", "--speed", "4"), 0, "1 error frames skipped")
	want(t, run(t, string(mustRead(t, trace)), "replay", "-", "-i", "virtual"), 0, "replayed 3")
	want(t, run(t, "", "replay", trace, "-i", "virtual", "--speed", "0"), 3, "--speed")
	want(t, run(t, "", "replay", trace, "-i", "virtual", "--filter", "q"), 3)
	want(t, run(t, "", "replay", "testdata/missing.log", "-i", "virtual"), 3)
	want(t, run(t, "", "replay", trace, "-i", "virtual", "--rate", "-1"), 3)
}

func mustRead(t *testing.T, p string) []byte {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStats(t *testing.T) {
	r := runWithTraffic(t, vehicleFrames(t), "stats", "-i", "virtual", "--duration", "100ms")
	want(t, r, 0, "live", "100 ", "18FEF1FE")
	r = runWithTraffic(t, vehicleFrames(t), "stats", "-i", "virtual", "--duration", "60ms", "--format", "table")
	want(t, r, 0, "RATE_HZ", "200")
	r = runWithTraffic(t, vehicleFrames(t), "stats", "-i", "virtual", "--duration", "60ms", "--format", "json")
	want(t, r, 0, `"ids"`)
	r = run(t, "", "stats", "-i", "vcan9")
	if linux() {
		want(t, r, 4)
	} else {
		want(t, r, 8)
	}
}

func TestPacerAndHelpers(t *testing.T) {
	if !strings.Contains(frameText(jsonOf(t, "123#R5")), "R5") || frameText(jsonOf(t, "123##2AA")) != "123##2AA" {
		t.Fatal(frameText(jsonOf(t, "123#R5")))
	}
}

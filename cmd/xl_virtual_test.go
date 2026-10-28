// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
	"github.com/otfabric/go-canbus/virtual"
)

// mixedLog is a candump log with CAN, CAN FD and CAN XL frames.
const mixedLog = `(1700000000.000000) can0 123#01
(1700000000.010000) can0 05242#81:01:12345678#AABB
(1700000000.020000) can0 456##1112233445566778899AABBCC
(1700000000.030000) can1 00243#80:00:00000000#CC
`

func mixedFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "mixed.log")
	if err := os.WriteFile(p, []byte(mixedLog), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// readAny returns the frames of a trace as text, CAN XL frames included.
func readAny(t *testing.T, path string) []string {
	t.Helper()
	fh, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	r, _, err := capture.NewReader(fh)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for {
		var f canbus.Frame
		var x canbus.XLFrame
		isXL, err := r.(capture.XLReader).ReadAny(&f, &x)
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if isXL {
			out = append(out, x.Interface+" "+x.String())
		} else {
			out = append(out, f.Interface+" "+f.String())
		}
	}
}

func TestTraceConvertXL(t *testing.T) {
	in := mixedFile(t)
	dir := t.TempDir()
	for _, ext := range []string{".pcapng", ".jsonl", ".log"} {
		out := filepath.Join(dir, "x"+ext)
		want(t, run(t, "", "trace", "convert", in, out), 0, "4 frames read, 4 written")
		got := strings.Join(readAny(t, out), "|")
		if got != "can0 123#01|can0 05242#81:01:12345678#AABB|can0 456##1112233445566778899AABBCC|can1 00243#80:00:00000000#CC" {
			t.Fatalf("%s: %s", ext, got)
		}
	}
	// pcap keeps no interface names: reported.
	want(t, run(t, "", "trace", "convert", in, filepath.Join(dir, "x.pcap")), 0, "4 written", "lost: 4 frames: interface name")
	// ASC cannot hold CAN XL: reported, never silent.
	want(t, run(t, "", "trace", "convert", in, filepath.Join(dir, "x.asc")), 0, "4 frames read, 2 written", "lost: 2 frames: CAN XL, not written")
	// Filters select CAN XL frames by priority.
	out := filepath.Join(dir, "f.jsonl")
	want(t, run(t, "", "trace", "convert", in, out, "--filter", "242:7FF"), 0, "1 written", "3 filtered out")
	if got := readAny(t, out); len(got) != 1 || !strings.Contains(got[0], "05242#") {
		t.Fatal(got)
	}
}

func TestTraceInfoXL(t *testing.T) {
	want(t, run(t, "", "trace", "info", mixedFile(t)), 0, "4 frames", "CAN FD 1, CAN XL 2", "XL 05242", "XL 00243")
}

func TestXLOnVirtual(t *testing.T) {
	want(t, run(t, "", "xl", "send", "-i", "virtual", "00242#80:00:00000000#11", "--repeat", "2"), 0, "sent 2 CAN XL")
	// replay without --xl counts the CAN XL frames it cannot send.
	in := mixedFile(t)
	want(t, run(t, "", "replay", "-i", "virtual", "--max-speed", in), 0, "replayed 2 of 4", "2 CAN XL frames not sent")
	want(t, run(t, "", "replay", "-i", "virtual", "--max-speed", "--xl", "--print", in), 0, "replayed 4 of 4", "virtual 05242#81:01:12345678#AABB")
}

// runWithXLTraffic runs a command on the virtual bus while CAN and CAN XL
// frames are sent to it until the command returns.
func runWithXLTraffic(t *testing.T, args ...string) result {
	t.Helper()
	resetVirtual()
	port, err := sharedVirtual().Open(virtual.WithXL())
	if err != nil {
		t.Fatal(err)
	}
	x, _ := canbus.ParseXLFrame("05242#81:01:12345678#AABB")
	f, _ := canbus.NewStandard(0x123, []byte{1})
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			_ = port.SendXL(context.Background(), &x)
			_ = port.Send(context.Background(), &f)
			time.Sleep(2 * time.Millisecond)
		}
	}()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
	close(done)
	return result{code, out.String(), errOut.String()}
}

func TestRecordAndMonitorXL(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "r.pcapng")
	want(t, runWithXLTraffic(t, "record", "--xl", "-i", "virtual", "-n", "6", "-o", out), 0, "recorded 6 frames")
	got := readAny(t, out)
	var xls int
	for _, g := range got {
		if strings.Contains(g, "05242#81:01:12345678#AABB") {
			xls++
		}
		if !strings.HasPrefix(g, "virtual ") {
			t.Fatalf("interface lost: %q", g)
		}
	}
	if len(got) != 6 || xls == 0 || xls == 6 {
		t.Fatalf("%d frames, %d CAN XL: %v", len(got), xls, got)
	}
	// Without --xl, record keeps only CAN and CAN FD frames.
	out = filepath.Join(dir, "c.log")
	want(t, runWithXLTraffic(t, "record", "-i", "virtual", "-n", "3", "-o", out), 0, "recorded 3 frames")
	for _, g := range readAny(t, out) {
		if strings.Contains(g, "#81:") {
			t.Fatalf("CAN XL recorded without --xl: %v", g)
		}
	}
	// xl monitor writes pcap.
	out = filepath.Join(dir, "m.pcap")
	want(t, runWithXLTraffic(t, "xl", "monitor", "-i", "virtual", "-n", "4", "-o", out), 0, "05242#81:01:12345678#AABB")
	if got := readAny(t, out); len(got) != 4 {
		t.Fatal(got)
	}
}

// SPDX-License-Identifier: MIT

//go:build linux

package cmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// xlIface returns $CANBUS_TEST_XL_IFACE (default vcanxl0), a vcan interface
// up with the CAN XL MTU. Without it the tests skip, unless
// CANBUS_REQUIRE_XL=1.
func xlIface(t *testing.T) string {
	t.Helper()
	name := os.Getenv("CANBUS_TEST_XL_IFACE")
	if name == "" {
		name = "vcanxl0"
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil || ifi.Flags&net.FlagUp == 0 || ifi.MTU < 76 {
		if os.Getenv("CANBUS_REQUIRE_XL") == "1" {
			t.Fatalf("no usable %s", name)
		}
		t.Skipf("no usable %s (vcan, up, mtu 2060)", name)
	}
	return name
}

// rcvLists counts the kernel's CAN receivers (/proc/net/can/rcvlist_*).
func rcvLists() int {
	n := 0
	for _, l := range []string{"all", "fil", "inv", "eff", "sff", "err"} {
		d, err := os.ReadFile("/proc/net/can/rcvlist_" + l)
		if err != nil {
			return -1
		}
		for _, line := range strings.Split(string(d), "\n") {
			if f := strings.Fields(line); len(f) >= 6 && strings.HasPrefix(line, "  ") && !strings.HasPrefix(f[0], "(") && f[0] != "device" {
				n++
			}
		}
	}
	return n
}

// runAsync runs canbusctl in process in the background and returns once
// it has bound a CAN socket (a receiver appears in the kernel's lists).
func runAsync(t *testing.T, args ...string) chan result {
	t.Helper()
	before := rcvLists()
	ch := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
		ch <- result{code, out.String(), errOut.String()}
	}()
	for end := time.Now().Add(10 * time.Second); before >= 0 && rcvLists() <= before; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatal("xl monitor did not bind its socket")
		}
	}
	return ch
}

// TestVCANXL: xl monitor (text, JSON, both trace formats) receives what xl
// send and send transmit, in process.
func TestVCANXL(t *testing.T) {
	ifc := xlIface(t)
	frames := []string{"00242#80:01:00000000#AABBCC", "07123#83:02:DEADBEEF#" + strings.Repeat("5A", 300)}
	dir := t.TempDir()
	for _, c := range []struct {
		format, output string
	}{{"text", "xl.log"}, {"json", "xl.jsonl"}, {"text", ""}} {
		args := []string{"--format", c.format, "xl", "monitor", "-i", ifc, "-n", "3", "--duration", "10s"}
		path := ""
		if c.output != "" {
			path = filepath.Join(dir, c.output)
			args = append(args, "-o", path)
		}
		mon := runAsync(t, args...)
		want(t, run(t, "", append([]string{"xl", "send", "-i", ifc, "--gap", "1ms"}, frames...)...), 0, "sent 2 CAN XL frame(s)")
		want(t, run(t, "", "send", "-i", ifc, "123#11"), 0)
		r := <-mon
		if c.format == "json" {
			want(t, r, 0, `"kind":"xl"`, `"priorityHex":"242"`, `"vcid":7`, `"rrs":true`, `"idHex":"123"`, `"type":"summary"`, `"xl":2`)
		} else {
			want(t, r, 0, frames[0], frames[1][:40], "123#11")
			if !strings.Contains(r.errOut, "3 frames (2 CAN XL), 0 dropped") {
				t.Fatalf("summary: %s", r.errOut)
			}
		}
		if path != "" {
			b, err := os.ReadFile(path)
			if err != nil || len(b) == 0 {
				t.Fatalf("%s: %v", path, err)
			}
		}
	}
	// JSON result of xl send; --duration ending a paced send early.
	want(t, run(t, "", "--format", "json", "xl", "send", "-i", ifc, frames[0]), 0, `"command": "xl send"`, `"priorityHex": "242"`)
	want(t, run(t, "", "xl", "send", "-i", ifc, "--repeat", "100", "--gap", "50ms", "--duration", "120ms", frames[0]), 0, "sent ")
	// --duration ends a monitor without frames.
	want(t, run(t, "", "xl", "monitor", "-i", ifc, "--duration", "100ms"), 0)
	// An interface without the CAN XL MTU.
	if ifi, err := net.InterfaceByName("vcan0"); err == nil && ifi.MTU < 76 {
		want(t, run(t, "", "xl", "monitor", "-i", "vcan0", "-n", "1"), 8, "CAN XL")
	}
}

// SPDX-License-Identifier: MIT

//go:build linux

package e2e

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// vcanXL returns $CANBUS_TEST_XL_IFACE (default vcanxl0), a vcan interface
// up with the CAN XL MTU. Without it the test skips, unless
// CANBUS_REQUIRE_XL=1.
func vcanXL(t *testing.T) string {
	t.Helper()
	name := os.Getenv("CANBUS_TEST_XL_IFACE")
	if name == "" {
		name = "vcanxl0"
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil || ifi.Flags&net.FlagUp == 0 || ifi.MTU < 76 {
		if os.Getenv("CANBUS_REQUIRE_XL") == "1" {
			t.Fatalf("no usable %s (vcan, up, mtu 2060)", name)
		}
		t.Skipf("no usable %s (vcan, up, mtu 2060; Linux ≥ 6.2)", name)
	}
	return name
}

// TestXLSendMonitor: one canbusctl process sends CAN XL frames, another
// prints them (with a CAN frame in between, which xl monitor shows too)
// and records them to a candump log and a JSON Lines trace.
func TestXLSendMonitor(t *testing.T) {
	ifc := vcanXL(t)
	dir := t.TempDir()
	frames := []string{"00242#80:01:00000000#AABBCC", "42123#81:02:DEADBEEF#" + strings.Repeat("5A", 600), "00001#83:FF:FFFFFFFF#00"}
	for _, out := range []string{"xl.log", "xl.jsonl"} {
		path := filepath.Join(dir, out)
		mon := start(t, "xl", "monitor", "-i", ifc, "-n", "4", "--duration", "20s", "-o", path)
		want(t, run(t, "", append([]string{"xl", "send", "-i", ifc}, frames[:2]...)...), 0, "sent 2 CAN XL frame(s)")
		want(t, run(t, "", "send", "-i", ifc, "123#11"), 0)
		want(t, run(t, "", "xl", "send", "-i", ifc, frames[2]), 0)
		r := mon.wait(t, 30*time.Second)
		want(t, r, 0, frames[0], frames[1][:30], "123#11", frames[2])
		if !strings.Contains(r.err, "4 frames (3 CAN XL), 0 dropped") {
			t.Fatalf("summary: %s", r.err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(b)
		if out == "xl.log" {
			for _, f := range frames {
				if !strings.Contains(text, " "+f+"\n") {
					t.Fatalf("%s lacks %.40s…:\n%.300s", out, f, text)
				}
			}
		} else if strings.Count(text, `"type":"xlframe"`) != 3 || strings.Count(text, `"type":"frame"`) != 1 {
			t.Fatalf("%s:\n%.300s", out, text)
		}
	}
	// JSON output of xl monitor.
	mon := start(t, "--format", "json", "xl", "monitor", "-i", ifc, "-n", "1", "--duration", "20s")
	want(t, run(t, "", "xl", "send", "-i", ifc, frames[0]), 0)
	want(t, mon.wait(t, 30*time.Second), 0, `"kind":"xl"`, `"priorityHex":"242"`, `"data":"aabbcc"`, `"type":"summary"`)
}

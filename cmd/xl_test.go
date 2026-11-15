// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otfabric/go-canbus"
)

func TestXLRefusals(t *testing.T) {
	want(t, run(t, "", "xl", "monitor", "-i", "slcan:/dev/null", "-n", "1"), 8, "SocketCAN interface")
	want(t, run(t, "", "record", "--xl", "-i", "slcan:/dev/null", "-o", filepath.Join(t.TempDir(), "x.log")), 8, "slcan adapters do not carry CAN XL")
	want(t, run(t, "", "record", "--xl", "-i", "virtual", "-o", "x.asc"), 3, "cannot hold CAN XL")
	want(t, run(t, "", "xl", "send", "-i", "vcanxl0", "--yes", "123#11"), 3, "CAN XL header")
	want(t, run(t, "", "xl", "send", "-i", "vcanxl0", "--yes", "00800#80:00:00000000#11"), 3, "priority")
	want(t, run(t, "", "xl", "send", "-i", "vcanxl0", "--yes", "--repeat", "0", "00242#80:00:00000000#11"), 3, "--repeat")
	want(t, run(t, "", "xl", "send", "-i", "vcanxl0", "--yes", "--max-frames", "1", "--repeat", "2", "00242#80:00:00000000#11"), 3, "--max-frames")
	want(t, run(t, "", "xl", "send", "-i", "vcanxl0"), 2)
	want(t, run(t, "", "xl", "monitor", "-i", "vcanxl0", "-o", "x.blf"), 3, "cannot hold CAN XL")
	// The interface is opened before the output is created (it opens here).
	want(t, run(t, "", "xl", "monitor", "-i", "virtual", "-o", "/nonexistent/x.log"), 3, "cannot create")
	if !linux() {
		want(t, run(t, "", "xl", "monitor", "-i", "vcanxl0", "-n", "1"), 8, "SocketCAN is not available")
	}
	// An existing output is left alone when the interface does not open
	// (it was truncated first).
	keep := filepath.Join(t.TempDir(), "keep.log")
	if err := os.WriteFile(keep, []byte("(1700000000.000000) can0 123#00\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := run(t, "", "xl", "monitor", "-i", "nosuchxl0", "-o", keep, "-n", "1")
	if r.code != 4 && r.code != 8 {
		t.Fatalf("exit %d, want 4 (no interface) or 8 (no SocketCAN): %s", r.code, r.errOut)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != "(1700000000.000000) can0 123#00\n" {
		t.Fatalf("the output was changed: %q %v", b, err)
	}
	r = run(t, "", "xl", "--help")
	want(t, r, 0, "<vcid><prio>#<flags>:<sdt>:<af>#<data>")
}

func TestXLJSON(t *testing.T) {
	x := canbus.XLFrame{Priority: 0x242, VCID: 7, Flags: canbus.XLSEC | canbus.XLRRS, SDT: 3, AF: 0xC0FFEE, Data: []byte{0xAB, 0xCD}}
	j := toXLJSON(&x)
	if j.text() != x.String() || j.PriorityHex != "242" || !j.SEC || !j.RRS || j.Len != 2 || j.Data != "abcd" || j.Timestamp != "" {
		t.Fatalf("%+v %s", j, j.text())
	}
	if l := xlLine(&x); !strings.HasPrefix(l, "(-) - 07242#83:03:00C0FFEE#ABCD") {
		t.Fatal(l)
	}
	res := xlSendResult{Interface: "vcanxl0", Sent: 1, Frames: []xlJSON{j}}
	if !strings.Contains(res.Text(), "vcanxl0 07242#83") || !strings.Contains(res.Text(), "sent 1 CAN XL") {
		t.Fatal(res.Text())
	}
}

// SPDX-License-Identifier: MIT

//go:build interop

package interop

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
)

// xlIface is the CAN XL interface: $CANBUS_TEST_XL_IFACE or vcanxl0, up
// with the CAN XL MTU; missing, it fails with CANBUS_REQUIRE_XL=1, else
// skips.
func xlIface(t *testing.T) string {
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
		t.Skipf("no usable %s (vcan, up, mtu 2060)", name)
	}
	return name
}

func xlFixture(t *testing.T) ([]byte, []canbus.XLFrame) {
	t.Helper()
	src := fixture(t, "frames/xl.jsonl")
	var out []canbus.XLFrame
	for _, line := range bytes.Split(src, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var d frameDoc
		if err := json.Unmarshal(line, &d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d.xlFrame(t))
	}
	return src, out
}

func compareXL(t *testing.T, got, want []canbus.XLFrame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d CAN XL frames, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].String() != want[i].String() {
			t.Fatalf("frame %d: got %.60s…, want %.60s…", i, got[i].String(), want[i].String())
		}
	}
}

// TestCtlXLSendsStackReceives: canbusctl xl send → the stack's receive --xl.
func TestCtlXLSendsStackReceives(t *testing.T) {
	ifc := xlIface(t)
	for _, a := range rawAdapters {
		scenario(t, a, "canbusctl xl send", func(t *testing.T) {
			if !caps(t, a).Features["xl"] {
				skipf(t, "%s has no CAN XL (feature xl, contract 1.3)", a)
			}
			_, want := xlFixture(t)
			p := start(t, a, "receive", "--interface", ifc, "--xl", "--count", strconv.Itoa(len(want)), "--timeout-ms", "30000")
			args := []string{"xl", "send", "-i", ifc}
			for i := range want {
				args = append(args, want[i].String())
			}
			if out, code := ctl(t, nil, args...); code != 0 {
				t.Fatalf("canbusctl xl send: exit %d: %s", code, out)
			}
			evs, code := p.wait(t, 60*time.Second)
			var got []canbus.XLFrame
			for _, e := range evs {
				if e.Type == "frame" && e.Frame != nil {
					got = append(got, e.Frame.xlFrame(t))
				}
			}
			if code != 0 {
				t.Fatalf("%s receive: exit %d, %d frames", a, code, len(got))
			}
			compareXL(t, got, want)
		})
	}
}

// TestStackXLToCtlMonitor: the stack's send --xl → canbusctl xl monitor
// --format json.
func TestStackXLToCtlMonitor(t *testing.T) {
	ifc := xlIface(t)
	for _, a := range rawAdapters {
		scenario(t, a, "xl to canbusctl xl monitor", func(t *testing.T) {
			if !caps(t, a).Features["xl"] {
				skipf(t, "%s has no CAN XL (feature xl, contract 1.3)", a)
			}
			src, want := xlFixture(t)
			mon := ctlStart(t, "xl", "monitor", "-i", ifc, "-n", strconv.Itoa(len(want)), "--duration", "60s", "--format", "json")
			if out, code := container(t, a, src, "", "send", "--interface", ifc, "--xl", "--frames", "-"); code != 0 {
				t.Fatalf("%s send: exit %d: %s", a, code, out)
			}
			out, code := mon.wait(t, 70*time.Second)
			if code != 0 {
				t.Fatalf("xl monitor: exit %d: %s", code, out)
			}
			var got []canbus.XLFrame
			for _, r := range jsonRecords(t, out) {
				if str(r["type"]) != "frame" || str(r["kind"]) != "xl" {
					continue
				}
				var j struct {
					PriorityHex string `json:"priorityHex"`
					VCID        uint8  `json:"vcid"`
					SEC, RRS    bool
					SDT         uint8  `json:"sdt"`
					AF          uint32 `json:"af"`
					Len         int    `json:"len"`
					Data        string `json:"data"`
				}
				b, _ := json.Marshal(r)
				_ = json.Unmarshal(b, &j)
				prio, _ := strconv.ParseUint(j.PriorityHex, 16, 16)
				d := frameDoc{XL: true, Priority: uint16(prio), VCID: j.VCID, SEC: j.SEC, RRS: j.RRS, SDT: j.SDT, AF: j.AF, Len: j.Len, Data: j.Data}
				got = append(got, d.xlFrame(t))
			}
			compareXL(t, got, want)
		})
	}
}

// SPDX-License-Identifier: MIT

//go:build interop

package interop

import (
	"encoding/hex"
	"os"
	"strconv"
	"testing"
	"time"
)

var isotpSizes = []int{7, 8, 62, 500, 4095}

// pattern is the deterministic payload of --size n on both sides.
func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// kernelISOTPMissing skips when the canutils adapter reports no can-isotp
// module (exit 4).
func kernelISOTPMissing(t *testing.T, a string, code int) {
	t.Helper()
	if a == "canutils" && code == 4 {
		if os.Getenv("CANBUS_REQUIRE_ISOTP") == "1" {
			t.Fatal("kernel ISO-TP (can-isotp) is not available")
		}
		skipf(t, "kernel ISO-TP (can-isotp) is not available")
	}
}

// TestISOTP exchanges messages between canbusctl isotp (userspace) and
// the Linux kernel (canutils) and python-can-isotp, in both directions;
// and canbusctl --kernel against python-can-isotp.
func TestISOTP(t *testing.T) {
	ifc := needVCAN(t)
	for _, a := range isotpAdapters {
		scenario(t, a, "canbusctl isotp send → stack", func(t *testing.T) {
			if !caps(t, a).has("isotp-receive") {
				skipf(t, "%s has no isotp-receive", a)
			}
			args := []string{"isotp-receive", "--interface", ifc, "--tx-id", "7e8", "--rx-id", "7e0",
				"--count", strconv.Itoa(len(isotpSizes)), "--timeout-ms", "60000"}
			if a == "pythoncanisotp" {
				args = append(args, "--stmin", "1") // its Python receive path is slower than vcan
			}
			p := start(t, a, args...)
			for _, n := range isotpSizes {
				out, code := ctl(t, nil, "isotp", "send", "-i", ifc, "--tx-id", "7E0", "--rx-id", "7E8", "--size", strconv.Itoa(n), "--timeout", "20s")
				if code != 0 {
					evs, c := p.wait(t, 5*time.Second)
					kernelISOTPMissing(t, a, c)
					t.Fatalf("%d bytes: canbusctl exit %d: %s (%d events)", n, code, out, len(evs))
				}
			}
			evs, code := p.wait(t, 90*time.Second)
			kernelISOTPMissing(t, a, code)
			var got []string
			for _, e := range evs {
				if e.Type == "message" {
					got = append(got, e.Data)
				}
			}
			if code != 0 || len(got) != len(isotpSizes) {
				t.Fatalf("exit %d, %d of %d messages", code, len(got), len(isotpSizes))
			}
			for i, n := range isotpSizes {
				if got[i] != hex.EncodeToString(pattern(n)) {
					t.Fatalf("message %d (%d bytes) differs", i, n)
				}
			}
		})
		scenario(t, a, "stack → canbusctl isotp receive", func(t *testing.T) {
			if !caps(t, a).has("isotp-send") {
				skipf(t, "%s has no isotp-send", a)
			}
			for _, n := range isotpSizes {
				// The kernel sender drops a flow control that beats its
				// own echo on vcan: the userspace receiver waits 200 µs.
				rx := ctlStart(t, "isotp", "receive", "-i", ifc, "--tx-id", "7E8", "--rx-id", "7E0", "--fc-delay", "200us",
					"--timeout", "30s", "--format", "json")
				out, code := container(t, a, nil, "", "isotp-send", "--interface", ifc, "--tx-id", "7e0", "--rx-id", "7e8",
					"--size", strconv.Itoa(n), "--timeout-ms", "20000")
				kernelISOTPMissing(t, a, code)
				if code != 0 {
					t.Fatalf("%d bytes: %s isotp-send exit %d: %s", n, a, code, out)
				}
				got, code2 := rx.wait(t, 40*time.Second)
				recs := jsonRecords(t, got)
				if code2 != 0 || len(recs) == 0 || str(recs[0]["data"]) != hex.EncodeToString(pattern(n)) {
					t.Fatalf("%d bytes: canbusctl exit %d: %.200s", n, code2, got)
				}
			}
		})
	}
	scenario(t, "pythoncanisotp", "canbusctl --kernel send → python-can-isotp", func(t *testing.T) {
		if !caps(t, "pythoncanisotp").has("isotp-receive") {
			skipf(t, "pythoncanisotp has no isotp-receive")
		}
		p := start(t, "pythoncanisotp", "isotp-receive", "--interface", ifc, "--tx-id", "7e8", "--rx-id", "7e0",
			"--count", "1", "--stmin", "1", "--timeout-ms", "30000")
		out, code := ctl(t, nil, "isotp", "send", "-i", ifc, "--kernel", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "1000", "--timeout", "20s")
		if code == 8 || code == 4 {
			if os.Getenv("CANBUS_REQUIRE_ISOTP") == "1" {
				t.Fatalf("kernel ISO-TP: exit %d: %s", code, out)
			}
			skipf(t, "kernel ISO-TP (can-isotp) is not available")
		}
		if code != 0 {
			t.Fatalf("canbusctl --kernel: exit %d: %s", code, out)
		}
		evs, c := p.wait(t, 40*time.Second)
		var got string
		for _, e := range evs {
			if e.Type == "message" {
				got = e.Data
			}
		}
		if c != 0 || got != hex.EncodeToString(pattern(1000)) {
			t.Fatalf("exit %d, %d events", c, len(evs))
		}
	})
}

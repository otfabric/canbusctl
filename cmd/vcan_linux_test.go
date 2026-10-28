// SPDX-License-Identifier: MIT

//go:build linux

package cmd

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/isotp"
	"github.com/otfabric/go-canbus/socketcan"
)

// vcanIface returns $CANBUS_TEST_IFACE (default vcan0), which must be a
// vcan interface, up, with the CAN FD MTU. Without it the tests skip,
// unless CANBUS_REQUIRE_VCAN=1, where they fail.
func vcanIface(t *testing.T) string {
	t.Helper()
	name := os.Getenv("CANBUS_TEST_IFACE")
	if name == "" {
		name = "vcan0"
	}
	ifi, err := net.InterfaceByName(name)
	if err != nil || ifi.Flags&net.FlagUp == 0 || ifi.MTU < 72 {
		if os.Getenv("CANBUS_REQUIRE_VCAN") == "1" {
			t.Fatalf("no usable %s", name)
		}
		t.Skipf("no usable %s (vcan, up, mtu 72)", name)
	}
	return name
}

// trafficOn sends frames on the interface until the returned stop is
// called.
func trafficOn(t *testing.T, ifc string, frames []canbus.Frame) func() {
	c, err := socketcan.Open(ifc, socketcan.WithFD())
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			for i := range frames {
				select {
				case <-done:
					return
				default:
				}
				_ = c.Send(context.Background(), &frames[i])
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()
	return func() { close(done); <-finished; c.Close() }
}

func TestVCANInterfaces(t *testing.T) {
	ifc := vcanIface(t)
	want(t, run(t, "", "interfaces"), 0, ifc, "vcan")
	want(t, run(t, "", "interfaces", "--format", "table"), 0, "KIND", ifc)
	want(t, run(t, "", "info", ifc), 0, ifc+" (index", "kind vcan")
	want(t, run(t, "", "info", "--format", "json", ifc), 0, `"kind": "vcan"`)
	want(t, run(t, "", "info", "nosuchcan9"), 4)
	want(t, run(t, "", "info", "lo"), 8, "not a CAN interface")
	want(t, run(t, "", "stats", "-i", ifc), 0, "rx ")
	if os.Geteuid() != 0 {
		want(t, run(t, "", "configure", ifc, "--bitrate", "500000", "--yes"), 7, "CAP_NET_ADMIN")
		want(t, run(t, "", "up", ifc, "--yes"), 7)
	}
}

func TestVCANFrames(t *testing.T) {
	ifc := vcanIface(t)
	stop := trafficOn(t, ifc, vehicleFrames(t))
	r := run(t, "", "monitor", "-i", ifc, "-n", "3", "--dbc", vehicle, "--filter", "100:7FF,std", "--filter", "200:7FF,std", "--filter", "18FEF1FE:1FFFFFFF")
	want(t, r, 0, ifc+" 100#3412142000000000  Engine: Speed=582.5")
	out := filepath.Join(t.TempDir(), "rec.jsonl")
	want(t, run(t, "", "record", "-i", ifc, "-o", out, "-n", "5"), 0, "recorded 5")
	want(t, run(t, "", "stats", "-i", ifc, "--duration", "100ms"), 0, "live", "18FEF1FE")
	stop()

	// Sending and replaying on vcan needs no --yes.
	rx, _ := socketcan.Open(ifc, socketcan.WithFD(), socketcan.WithFilters(canbus.FilterSet{Filters: []canbus.Filter{canbus.ExactStandard(0x5A5)}}))
	defer rx.Close()
	want(t, run(t, "", "send", "-i", ifc, "5A5#0102"), 0, "sent 1")
	want(t, run(t, "", "send-fd", "-i", ifc, "5A5##1"+strings.Repeat("AB", 12)), 0, "sent 1")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var f canbus.Frame
	if err := rx.Receive(ctx, &f); err != nil || f.Len != 2 {
		t.Fatal(f, err)
	}
	if err := rx.Receive(ctx, &f); err != nil || !f.IsFD() || f.Len != 12 {
		t.Fatal(f, err)
	}
	want(t, run(t, "", "replay", out, "-i", ifc, "--max-speed"), 0, "replayed 5 of 5")
	want(t, run(t, "", "simulate", "--dbc", vehicle, "-i", ifc, "--max-frames", "3"), 0, "simulated 3")
	want(t, run(t, "", "inject", "-i", ifc, "--frame", "5A5#09", "--duplicate", "1"), 0, "2 delivered")
}

func TestVCANKernelISOTP(t *testing.T) {
	ifc := vcanIface(t)
	probe, err := socketcan.DialISOTP(ifc, isotp.Config{TxID: 0x7F0, RxID: 0x7F1})
	if err != nil {
		if errors.Is(err, syscall.EPROTONOSUPPORT) || strings.Contains(err.Error(), "protocol not supported") {
			if os.Getenv("CANBUS_REQUIRE_ISOTP") == "1" {
				t.Fatal(err)
			}
			t.Skip("no can-isotp module:", err)
		}
		t.Fatal(err)
	}
	probe.Close()
	resetVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rc := make(chan result, 1)
	go func() {
		var o, e bytes.Buffer
		code := Run(ctx, []string{"isotp", "receive", "-i", ifc, "--kernel", "--tx-id", "7E8", "--rx-id", "7E0", "--timeout", "10s"}, strings.NewReader(""), &o, &e)
		rc <- result{code, o.String(), e.String()}
	}()
	time.Sleep(200 * time.Millisecond)
	s := run(t, "", "isotp", "send", "-i", ifc, "--tx-id", "7E0", "--rx-id", "7E8", "--size", "300")
	want(t, s, 0, "sent 1")
	want(t, <-rc, 0, "300 bytes")

	// Kernel sender, userspace receiver granting blocks of 4: --fc-delay
	// keeps each flow control behind the kernel's echo of its last frame.
	go func() {
		var o, e bytes.Buffer
		code := Run(ctx, []string{"isotp", "receive", "-i", ifc, "--tx-id", "7E8", "--rx-id", "7E0", "--bs", "4", "--fc-delay", "200us",
			"-n", "3", "--timeout", "10s"}, strings.NewReader(""), &o, &e)
		rc <- result{code, o.String(), e.String()}
	}()
	time.Sleep(200 * time.Millisecond)
	s = run(t, "", "isotp", "send", "-i", ifc, "--kernel", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "2000", "-n", "3")
	want(t, s, 0, "sent 3")
	want(t, <-rc, 0, "2000 bytes")
}

// SPDX-License-Identifier: MIT

//go:build linux

package cmd

import (
	"bytes"
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/socketcan"
	"github.com/otfabric/go-canbus/virtual"
)

// secondVCAN returns $CANBUS_TEST_IFACE2 (default vcan1), like vcanIface.
func secondVCAN(t *testing.T) string {
	t.Helper()
	name := os.Getenv("CANBUS_TEST_IFACE2")
	if name == "" {
		name = "vcan1"
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

// startBridge runs canbusctl bridge in process until the returned wait.
func startBridge(t *testing.T, args ...string) func() result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	rc := make(chan result, 1)
	go func() {
		var o, e bytes.Buffer
		code := Run(ctx, append([]string{"bridge"}, args...), strings.NewReader(""), &o, &e)
		rc <- result{code, o.String(), e.String()}
	}()
	time.Sleep(200 * time.Millisecond)
	return func() result { defer cancel(); return <-rc }
}

func TestVCANBridgeVirtual(t *testing.T) {
	ifc := vcanIface(t)
	resetVirtual()
	port, _ := sharedVirtual().Open(virtual.WithFD())
	defer port.Close()
	sock, err := socketcan.Open(ifc, socketcan.WithFD(), socketcan.WithFilters(canbus.FilterSet{Filters: []canbus.Filter{canbus.ExactStandard(0x2A2)}}))
	if err != nil {
		t.Fatal(err)
	}
	defer sock.Close()
	wait := startBridge(t, "-a", ifc, "-b", "virtual", "--duration", "1s", "--format", "json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// vcan → virtual.
	tx, _ := socketcan.Open(ifc, socketcan.WithFD())
	defer tx.Close()
	in, _ := canbus.NewStandard(0x1A1, []byte{1, 2, 3})
	if err := tx.Send(ctx, &in); err != nil {
		t.Fatal(err)
	}
	var f canbus.Frame
	for {
		if err := port.Receive(ctx, &f); err != nil {
			t.Fatal(err)
		}
		if f.ID == 0x1A1 {
			break
		}
	}
	// virtual → vcan.
	back, _ := canbus.NewStandard(0x2A2, []byte{9})
	if err := port.Send(ctx, &back); err != nil {
		t.Fatal(err)
	}
	if err := sock.Receive(ctx, &f); err != nil || f.Data[0] != 9 {
		t.Fatal(f, err)
	}
	r := wait()
	want(t, r, 0)
	doc := jsonDoc(t, r.out)["result"].(map[string]any)
	if doc["aToB"].(map[string]any)["forwarded"].(float64) < 1 || doc["bToA"].(map[string]any)["forwarded"].(float64) != 1 {
		t.Fatal(r.out)
	}
}

func TestVCANBridgeFaults(t *testing.T) {
	a, b := vcanIface(t), secondVCAN(t)
	rx, err := socketcan.Open(b, socketcan.WithFD(), socketcan.WithFilters(canbus.FilterSet{Filters: []canbus.Filter{canbus.ExactStandard(0x3B3)}}))
	if err != nil {
		t.Fatal(err)
	}
	defer rx.Close()
	wait := startBridge(t, "-a", a, "-b", b, "--one-way", "--duration", "1500ms", "--drop", "0.3", "--duplicate", "0.2",
		"--delay-probability", "0.1", "--delay", "2ms", "--seed", "7")
	tx, _ := socketcan.Open(a)
	defer tx.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const n = 200
	for i := 0; i < n; i++ {
		f, _ := canbus.NewStandard(0x3B3, []byte{byte(i)})
		if err := tx.Send(ctx, &f); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Microsecond)
	}
	got := map[byte]int{}
	for {
		c, stop := context.WithTimeout(ctx, 300*time.Millisecond)
		var f canbus.Frame
		err := rx.Receive(c, &f)
		stop()
		if err != nil {
			break
		}
		got[f.Data[0]]++
	}
	r := wait()
	want(t, r, 0, "dropped", "duplicated")
	// Never wrong data: every delivered payload was sent, at most twice.
	if len(got) == 0 || len(got) >= n {
		t.Fatalf("%d distinct frames of %d delivered with 30 %% drops", len(got), n)
	}
	for v, c := range got {
		if int(v) >= n || c > 2 {
			t.Fatalf("payload %d delivered %d times", v, c)
		}
	}
}

func TestVCANMonitorOutput(t *testing.T) {
	ifc := vcanIface(t)
	stop := trafficOn(t, ifc, vehicleFrames(t))
	defer stop()
	out := t.TempDir() + "/m.asc"
	want(t, run(t, "", "monitor", "-i", ifc, "-n", "4", "-o", out), 0)
	if n := len(traceFrames(t, out)); n != 4 {
		t.Fatal(n)
	}
}

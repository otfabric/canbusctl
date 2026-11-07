// SPDX-License-Identifier: MIT

package cmd

import (
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// transmitting are the commands that transmit, each with what it needs
// besides the interface; {if} is the interface.
var transmitting = [][]string{
	{"send", "-i", "{if}", "123#00"},
	{"send-fd", "-i", "{if}", "123##0"},
	{"xl", "send", "-i", "{if}", "123#80:00:00000000#00"},
	{"replay", "-i", "{if}", "testdata/trace.log"},
	{"isotp", "send", "-i", "{if}", "--tx-id", "7E0", "--rx-id", "7E8", "--data", "01"},
	{"isotp", "send", "-i", "{if}", "--tx-id", "7E0", "--rx-id", "7E8", "--data", "01", "--kernel"},
	{"simulate", "-i", "{if}", "--dbc", "testdata/vehicle.dbc", "--duration", "100ms"},
	{"inject", "-i", "{if}", "--frame", "123#00"},
	{"bridge", "-a", "virtual", "-b", "{if}", "--duration", "100ms"},
	{"bridge", "-a", "{if}", "-b", "virtual", "--duration", "100ms"},
}

func withIface(args []string, iface string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.ReplaceAll(a, "{if}", iface)
	}
	return out
}

// TestTransmitNeedsYes: every transmitting command refuses an interface
// that is not virtual without --yes (exit 13, confirmation_required)
// before it opens it: a serial-line adapter behind a TCP serial server
// is never even connected to.
func TestTransmitNeedsYes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var accepted atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			_ = c.Close()
		}
	}()
	ifaces := []string{"slcan:tcp://" + ln.Addr().String(), "slcan:/dev/canbusctl-test-none"}
	if linux() {
		ifaces = append(ifaces, "lo") // not a CAN interface: physical or unknown
	}
	for _, iface := range ifaces {
		for _, c := range transmitting {
			args := append(withIface(c, iface), "--format", "json")
			want(t, run(t, "", args...), 13, `"code": "confirmation_required"`, "--yes")
		}
	}
	for _, c := range [][]string{{"up", "vcan0"}, {"down", "vcan0"}, {"up", "virtual"}, {"configure", "vcan0", "--mtu", "72"},
		{"configure", "can0", "--restart"}, {"configure", "can0", "--bitrate", "500000", "--fd"}} {
		want(t, run(t, "", append(c, "--format", "json")...), 13, `"code": "confirmation_required"`)
	}
	time.Sleep(50 * time.Millisecond)
	if n := accepted.Load(); n != 0 {
		t.Fatalf("%d connections to the adapter without --yes", n)
	}
	// With --yes the check passes and the adapter is opened.
	r := run(t, "", "send", "-i", "slcan:tcp://"+ln.Addr().String(), "--yes", "123#00")
	if r.code == 13 {
		t.Fatalf("--yes refused: %s", r.errOut)
	}
	for end := time.Now().Add(2 * time.Second); accepted.Load() == 0 && time.Now().Before(end); {
		time.Sleep(10 * time.Millisecond)
	}
	if accepted.Load() == 0 {
		t.Fatal("the listener saw no connection with --yes: the check above proves nothing")
	}
}

// TestPassiveCommandsNeedNoYes: the passive commands do not need
// --yes, and isotp monitor refuses --kernel rather than opening a
// socket that would send flow control.
func TestPassiveCommandsNeedNoYes(t *testing.T) {
	for _, c := range [][]string{{"monitor", "-i", "virtual", "--duration", "10ms"}, {"record", "-i", "virtual", "-o", t.TempDir() + "/r.log", "--duration", "10ms"},
		{"stats", "-i", "virtual", "--duration", "10ms"}, {"isotp", "monitor", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--duration", "10ms"}} {
		want(t, run(t, "", c...), 0)
	}
	want(t, run(t, "", "isotp", "monitor", "-i", "vcan0", "--tx-id", "7E0", "--rx-id", "7E8", "--kernel"), 8, "--kernel")
}

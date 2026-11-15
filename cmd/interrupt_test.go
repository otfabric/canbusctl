// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/virtual"
)

// TestInterrupted: what each long-running command does when its context
// is cancelled (SIGINT/SIGTERM in Execute). Streams and transmissions
// with a partial result end with 0 and print it; a command without a
// result ends with 130 (canceled). isotp receive --count 1 ended with 5
// (timeout) although no --timeout was given. The context is cancelled
// once the command waits: a receiving command (and inject, whose fault
// stage holds the frame) when it is ready (readyHook), a transmitting one
// when its first frame is on the bus (or on the sink).
func TestInterrupted(t *testing.T) {
	gap := writeFile(t, "gap.log", "(1700000000.000000) can0 100#11\n(1700000100.000000) can0 100#22\n")
	rec := filepath.Join(t.TempDir(), "r.log")
	for _, c := range []struct {
		args  []string
		ready string // the readyHook name; "" waits for the first frame sent, "sink" for the first on sink0
		code  int
		out   string
	}{
		{[]string{"monitor", "-i", "virtual"}, "monitor", 0, ""},
		{[]string{"record", "-i", "virtual", "-o", rec}, "record", 0, "recorded 0 frames"},
		{[]string{"stats", "-i", "virtual", "--duration", "1m"}, "stats", 0, "live"},
		{[]string{"xl", "monitor", "-i", "virtual"}, "xl monitor", 0, ""},
		{[]string{"isotp", "monitor", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8"}, "isotp monitor", 0, ""},
		{[]string{"isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "0"}, "isotp receive", 0, ""},
		{[]string{"isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "1", "--format", "json"}, "isotp receive", 130, `"code": "canceled"`},
		{[]string{"isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "100"}, "", 130, "canceled"},
		{[]string{"send", "-i", "virtual", "--gap", "1m", "123#00", "123#01"}, "", 0, "sent 1 frame(s)"},
		{[]string{"send-fd", "-i", "virtual", "--gap", "1m", "--repeat", "3", "123##0"}, "", 0, "sent 1 frame(s)"},
		{[]string{"xl", "send", "-i", "virtual", "--gap", "1m", "123#80:00:00000000#00", "123#80:00:00000000#01"}, "", 0, "sent 1 CAN XL frame(s)"},
		// Read is 1 or 2: the cancel may come before the second record is read.
		{[]string{"replay", "-i", "virtual", gap}, "", 0, "replayed 1 of "},
		{[]string{"replay", "-i", "virtual", "--print", gap}, "", 0, "replayed 1 of "},
		{[]string{"inject", "-i", "sink0", "--yes", "--rate", "0.001", "--frame", "123#00", "--frame", "124#00"}, "sink", 0, "2 source frames, 1 delivered to sink0"},
		{[]string{"simulate", "--dbc", "testdata/vehicle.dbc", "--duration", "1m"}, "", 0, "Engine"},
		{[]string{"inject", "--frame", "123#00", "--delay-probability", "1", "--delay", "1m"}, "inject", 0, "1 delayed"},
	} {
		resetVirtual()
		ctx, cancel := context.WithCancel(context.Background())
		waiting := make(chan struct{})
		switch c.ready {
		case "sink":
			waiting = withSink(t).first
		case "":
			spy, err := sharedVirtual().Open(virtual.WithFD(), virtual.WithXL())
			if err != nil {
				t.Fatal(err)
			}
			go func() {
				var f canbus.Frame
				var x canbus.XLFrame
				if _, err := spy.ReceiveAny(ctx, &f, &x); err == nil {
					close(waiting)
				}
			}()
		default:
			onReady(t, c.ready, func() { close(waiting) })
		}
		interrupted := make(chan struct{})
		go func() {
			defer close(interrupted)
			select {
			case <-waiting:
				cancel()
			case <-ctx.Done():
			}
		}()
		var out, errOut bytes.Buffer
		watchdog := time.AfterFunc(30*time.Second, cancel) // a failure bound: the command never got ready
		began := time.Now()
		code := Run(ctx, c.args, strings.NewReader(""), &out, &errOut)
		watchdog.Stop()
		select {
		case <-waiting:
		default:
			t.Fatalf("%q ended (exit %d) before it waited: %s%s", c.args, code, out.String(), errOut.String())
		}
		cancel()
		<-interrupted
		if el := time.Since(began); el > 5*time.Second {
			t.Fatalf("%q took %v after the interrupt", c.args, el)
		}
		want(t, result{code, out.String(), errOut.String()}, c.code, c.out)
	}
	// --timeout still ends isotp receive --count with 5.
	want(t, run(t, "", "isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "1", "--timeout", "50ms"), 5, "received 0 of 1")
}

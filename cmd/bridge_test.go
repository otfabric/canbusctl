// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/chaos"
)

func TestBridgeErrors(t *testing.T) {
	want(t, run(t, "", "bridge", "-a", "virtual"), 2, "side-b")
	want(t, run(t, "", "bridge", "-a", "virtual", "-b", "virtual", "--duration", "1s"), 3, "different")
	want(t, run(t, "", "bridge", "-a", "virtual", "-b", "vcan0"), 3, "--duration")
	want(t, run(t, "", "bridge", "-a", "virtual", "-b", "vcan0", "--duration", "1s", "--drop", "1.5"), 3, "between 0 and 1")
	want(t, run(t, "", "bridge", "-a", "virtual", "-b", "vcan0", "--duration", "1s", "--filter", "zz"), 3)
	if linux() { // elsewhere the interface kind cannot be determined (exit 8)
		want(t, run(t, "", "bridge", "-a", "virtual", "-b", "lo", "--duration", "1s"), 9, "--yes")
		want(t, run(t, "", "bridge", "-a", "lo", "-b", "virtual", "--duration", "1s"), 9, "--yes")
	} else {
		want(t, run(t, "", "bridge", "-a", "virtual", "-b", "vcan0", "--duration", "1s"), 8)
		want(t, run(t, "", "bridge", "-a", "vcan0", "-b", "virtual", "--duration", "1s", "--one-way", "--filter", "100:7FF"), 8)
	}
}

func TestBridgeResultText(t *testing.T) {
	r := bridgeResult{A: "vcan0", B: "vcan1", DurationS: 1, AToB: chaos.Stats{Received: 3, Forwarded: 2, Dropped: 1}}
	if s := r.Text(); !strings.Contains(s, "vcan0 → vcan1: 3 received, 2 forwarded: 1 dropped") || !strings.Contains(s, "vcan1 → vcan0") {
		t.Fatal(s)
	}
	r.OneWay = true
	if strings.Contains(r.Text(), "vcan1 → vcan0") {
		t.Fatal(r.Text())
	}
}

func TestInjectReorder(t *testing.T) {
	r := run(t, "", "inject", "--frame", "001#01", "--frame", "002#02", "--frame", "003#03", "--reorder", "1", "--seed", "3")
	want(t, r, 0, "3 delivered", "reordered")
	if strings.Index(r.out, "virtual 001#01") < strings.Index(r.out, "virtual 002#02") {
		t.Fatal("the first frame was not held back:\n" + r.out)
	}
}

func TestEndedEarlyDeadline(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	// The context's timer has fired by now; a deadline error from a
	// socket is a normal end, other errors are not.
	if !ended(ctx, os.ErrDeadlineExceeded) || !ended(ctx, canbus.ErrTimeout) || ended(ctx, canbus.ErrClosed) {
		t.Fatal("ended")
	}
	if ended(context.Background(), canbus.ErrTimeout) {
		t.Fatal("no deadline")
	}
}

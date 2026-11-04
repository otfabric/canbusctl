// SPDX-License-Identifier: MIT

//go:build linux

package cmd

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/socketcan"
)

// TestVCANChaosBridgeSeeds is TestChaosBridgeSeeds on vcan0 → vcan1
// (one way, real sockets): per seed and fault set, every frame sent on
// vcan0 is received by the bridge, forwarded = received - dropped +
// duplicated and exactly that many frames reach vcan1, none invented,
// none more than twice, payloads intact without --corrupt, the order
// kept without --reorder; the bridge returns within --duration + 2 s
// and leaves no goroutine. Seeds: $CANBUS_CHAOS_ITERATIONS (default 3).
func TestVCANChaosBridgeSeeds(t *testing.T) {
	a, b := vcanIface(t), secondVCAN(t)
	const n = 40
	src := sourceFrames(n)
	before := goroutines()
	var total faultTotals
	for _, fs := range chaosFaultSets {
		for seed := 1; seed <= chaosIterations(3); seed++ {
			t.Run(fmt.Sprintf("%s/seed=%d", fs.name, seed), func(t *testing.T) {
				rx, err := socketcan.Open(b, socketcan.WithFilters(canbus.FilterSet{Filters: []canbus.Filter{{ID: 0x100, Mask: 0x7C0}}}))
				if err != nil {
					t.Fatal(err)
				}
				defer rx.Close()
				tx, err := socketcan.Open(a)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Close()
				began := time.Now()
				wait := startBridge(t, append([]string{"-a", a, "-b", b, "--one-way", "--duration", "800ms", "--format", "json", "--seed", strconv.Itoa(seed)}, fs.flags...)...)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				for i := range src {
					if err := tx.Send(ctx, &src[i]); err != nil {
						t.Fatal(err)
					}
				}
				var got []chaosFrame
				for {
					c, stop := context.WithTimeout(ctx, 300*time.Millisecond)
					var f canbus.Frame
					err := rx.Receive(c, &f)
					stop()
					if err != nil {
						break
					}
					got = append(got, chaosFrame{uint32(f.ID), slices.Clone(f.Payload())})
				}
				r := wait()
				if el := time.Since(began); el > 800*time.Millisecond+2*time.Second {
					t.Fatalf("bridge took %v", el)
				}
				want(t, r, 0)
				st := statsOf(t, jsonDoc(t, r.out)["result"].(map[string]any)["aToB"])
				checkStats(t, fmt.Sprintf("seed %d %v", seed, fs.flags), st, n)
				total.add(st)
				if uint64(len(got)) != st.Forwarded {
					t.Fatalf("seed %d %v: %d frames arrived, %d forwarded", seed, fs.flags, len(got), st.Forwarded)
				}
				checkDelivered(t, fs, src, got)
			})
		}
	}
	total.check(t)
	noLeak(t, before)
}

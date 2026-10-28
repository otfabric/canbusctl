// SPDX-License-Identifier: MIT

//go:build linux

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// vcan returns $<env> (default def), a vcan interface that is up with the
// CAN FD MTU; missing, it fails with CANBUS_REQUIRE_VCAN=1, else skips.
func vcan(t *testing.T, env, def string) string {
	t.Helper()
	name := os.Getenv(env)
	if name == "" {
		name = def
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

func vcan0(t *testing.T) string { return vcan(t, "CANBUS_TEST_IFACE", "vcan0") }
func vcan1(t *testing.T) string { return vcan(t, "CANBUS_TEST_IFACE2", "vcan1") }

// kernelISOTP skips (or fails with CANBUS_REQUIRE_ISOTP=1) without the
// can-isotp module.
func kernelISOTP(t *testing.T, ifc string) {
	t.Helper()
	r := run(t, "", "isotp", "receive", "-i", ifc, "--kernel", "--tx-id", "7F1", "--rx-id", "7F0", "--timeout", "10ms", "-n", "0")
	if r.code != 0 {
		if os.Getenv("CANBUS_REQUIRE_ISOTP") == "1" {
			t.Fatalf("kernel ISO-TP: %s", r.err)
		}
		t.Skipf("no kernel ISO-TP: %s", strings.TrimSpace(r.err))
	}
}

// TestVCANPipeline: one process replays a trace on vcan while another
// records it; the recording goes through every format and is replayed
// again under a monitor; every stage decodes alike.
func TestVCANPipeline(t *testing.T) {
	ifc := vcan0(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "src.log")
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("(1700000000.%06d) can0 100#%02X12142000000000", i*1000, i),
			fmt.Sprintf("(1700000000.%06d) can0 18FEF1FE#01D0%02X0000000000", i*1000+500, i))
	}
	_ = os.WriteFile(src, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	base := decodeSignals(t, vehicle, src)
	for _, ext := range []string{".blf", ".asc", ".pcap", ".jsonl", ".log"} {
		rec := filepath.Join(dir, "rec"+ext)
		p := start(t, "record", "-i", ifc, "-o", rec, "-n", strconv.Itoa(len(lines)), "--duration", "30s", "--filter", "100:7FF,std", "--filter", "18FEF1FE:1FFFFFFF")
		want(t, run(t, "", "replay", src, "-i", ifc, "--max-speed"), 0, fmt.Sprintf("replayed %d of %d", len(lines), len(lines)))
		want(t, p.wait(t, 40*time.Second), 0, fmt.Sprintf("recorded %d frames", len(lines)))
		if got := decodeSignals(t, vehicle, rec); strings.Join(got, "\n") != strings.Join(base, "\n") {
			t.Fatalf("%s decodes differently", rec)
		}
		// Convert to the next format and replay that under a monitor.
		conv := filepath.Join(dir, "conv"+ext+".blf")
		want(t, run(t, "", "trace", "convert", rec, conv), 0)
		mon := start(t, "monitor", "-i", ifc, "-n", strconv.Itoa(len(lines)), "--duration", "30s", "--format", "json", "--dbc", vehicle,
			"--filter", "100:7FF,std", "--filter", "18FEF1FE:1FFFFFFF")
		want(t, run(t, "", "replay", conv, "-i", ifc, "--max-speed"), 0)
		r := mon.wait(t, 40*time.Second)
		want(t, r, 0)
		var got []string
		for _, l := range strings.Split(strings.TrimSpace(r.out), "\n") {
			var m map[string]json.RawMessage
			_ = json.Unmarshal([]byte(l), &m)
			if string(m["type"]) == `"frame"` {
				got = append(got, string(m["idHex"])+string(m["data"])+string(m["signals"]))
			}
		}
		if strings.Join(got, "\n") != strings.Join(base, "\n") {
			t.Fatalf("monitor of the %s replay decodes differently:\n%s", ext, strings.Join(got, "\n"))
		}
	}
}

// TestVCANISOTPProcesses: ISO-TP between two canbusctl processes, and
// between canbusctl and the kernel in both directions.
func TestVCANISOTPProcesses(t *testing.T) {
	ifc := vcan0(t)
	exchange := func(t *testing.T, recvExtra, sendExtra []string, size int) {
		t.Helper()
		rx := start(t, append([]string{"isotp", "receive", "-i", ifc, "--tx-id", "7E8", "--rx-id", "7E0", "-n", "2", "--timeout", "20s", "--format", "json"}, recvExtra...)...)
		want(t, run(t, "", append([]string{"isotp", "send", "-i", ifc, "--tx-id", "7E0", "--rx-id", "7E8", "--size", strconv.Itoa(size), "-n", "2"}, sendExtra...)...), 0, "sent 2")
		r := rx.wait(t, 30*time.Second)
		want(t, r, 0, fmt.Sprintf(`"len":%d`, size))
		if strings.Count(r.out, `"type":"message"`) != 2 {
			t.Fatal(r.out)
		}
	}
	t.Run("userspace", func(t *testing.T) { exchange(t, []string{"--bs", "8", "--stmin", "1ms"}, nil, 4095) })
	t.Run("userspace fd", func(t *testing.T) { exchange(t, []string{"--fd"}, []string{"--fd", "--brs"}, 3000) })
	t.Run("kernel receiver", func(t *testing.T) {
		kernelISOTP(t, ifc)
		exchange(t, []string{"--kernel"}, nil, 2000)
	})
	t.Run("kernel sender", func(t *testing.T) {
		kernelISOTP(t, ifc)
		exchange(t, []string{"--bs", "4", "--fc-delay", "200us"}, []string{"--kernel"}, 2000)
	})
}

// TestVCANBridgeProcesses: a bridge process between vcan0 and vcan1
// carries frames and ISO-TP between processes on either side.
func TestVCANBridgeProcesses(t *testing.T) {
	a, b := vcan0(t), vcan1(t)
	br := start(t, "bridge", "-a", a, "-b", b, "--duration", "4s", "--format", "json")
	mon := start(t, "monitor", "-i", b, "-n", "3", "--duration", "3s", "--filter", "5A1:7FF")
	want(t, run(t, "", "send", "-i", a, "5A1#01", "5A1#02", "5A1#03"), 0)
	want(t, mon.wait(t, 5*time.Second), 0, b+" 5A1#01", "5A1#03")
	rx := start(t, "isotp", "receive", "-i", b, "--tx-id", "7E8", "--rx-id", "7E0", "--timeout", "3s")
	want(t, run(t, "", "isotp", "send", "-i", a, "--tx-id", "7E0", "--rx-id", "7E8", "--size", "1000"), 0)
	want(t, rx.wait(t, 5*time.Second), 0, "1000 bytes")
	r := br.wait(t, 10*time.Second)
	want(t, r, 0, `"aToB"`, `"bToA"`)
}

// TestChaosISOTPThroughBridge: ISO-TP through a bridge that drops,
// duplicates and delays frames (seeded, several seeds) never delivers
// wrong data: the receiver reports the exact message or an error. With
// every fault, reordering and corruption included, every process still
// returns by its deadline.
func TestChaosISOTPThroughBridge(t *testing.T) {
	a, b := vcan0(t), vcan1(t)
	payload := pattern(200)
	seeds := 6
	if testing.Short() {
		seeds = 3
	}
	for seed := 1; seed <= seeds; seed++ {
		for _, all := range []bool{false, true} {
			name := fmt.Sprintf("seed %d", seed)
			faults := []string{"--drop", "0.005", "--duplicate", "0.01", "--delay-probability", "0.05", "--delay", "3ms"}
			if all {
				name += " all faults"
				faults = append(faults, "--reorder", "0.05", "--corrupt", "0.05")
			}
			t.Run(name, func(t *testing.T) {
				br := start(t, append([]string{"bridge", "-a", a, "-b", b, "--duration", "6s", "--seed", strconv.Itoa(seed)}, faults...)...)
				rx := start(t, "isotp", "receive", "-i", b, "--tx-id", "7E8", "--rx-id", "7E0", "-n", "3", "--timeout", "4s", "--format", "json")
				began := time.Now()
				s := run(t, "", "isotp", "send", "-i", a, "--tx-id", "7E0", "--rx-id", "7E8", "--size", "200", "-n", "3", "--timeout", "4s")
				r := rx.wait(t, 6*time.Second)
				if el := time.Since(began); el > 6*time.Second {
					t.Fatalf("ISO-TP endpoints took %v", el)
				}
				delivered := 0
				for _, l := range strings.Split(strings.TrimSpace(r.out), "\n") {
					var m map[string]any
					if json.Unmarshal([]byte(l), &m) != nil || m["type"] != "message" {
						continue
					}
					if !all && m["data"] != fmt.Sprintf("%x", payload) {
						t.Fatalf("wrong data delivered (%v bytes)", m["len"])
					}
					delivered++
				}
				chaosDelivered[all] += delivered
				t.Logf("%d of 3 delivered; sender exit %d, receiver exit %d", delivered, s.code, r.code)
				want(t, br.wait(t, 9*time.Second), 0)
			})
		}
	}
	if chaosDelivered[false] == 0 {
		t.Fatal("no message got through the light faults")
	}
}

// chaosDelivered counts the messages delivered under faults: with the
// light faults some must get through, or the test proves nothing.
var chaosDelivered = map[bool]int{}

func pattern(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

// proc is a canbusctl process in the background.
type proc struct {
	cmd  *exec.Cmd
	o, e bytes.Buffer
	done chan struct{}
	code int
}

func start(t *testing.T, args ...string) *proc {
	t.Helper()
	p := &proc{done: make(chan struct{})}
	p.cmd = exec.Command(bin, args...)
	p.cmd.Stdout, p.cmd.Stderr = &p.o, &p.e
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		if ee, ok := p.cmd.Wait().(*exec.ExitError); ok {
			p.code = ee.ExitCode()
		}
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	time.Sleep(300 * time.Millisecond) // bound by then
	return p
}

func (p *proc) wait(t *testing.T, d time.Duration) result {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(d):
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Fatalf("did not return within %v: %s %s", d, p.o.String(), p.e.String())
	}
	return result{p.code, p.o.String(), p.e.String()}
}

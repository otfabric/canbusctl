// SPDX-License-Identifier: MIT

//go:build !windows

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// signalled starts canbusctl, sends sig once it runs, and returns its
// result; the process must end within 3 s of the signal.
func signalled(t *testing.T, sig os.Signal, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(500 * time.Millisecond) // past signal.NotifyContext and the command's setup
	if err := cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	var err error
	select {
	case err = <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("%q did not end within 3 s of %v: %s %s", args, sig, o.String(), e.String())
	}
	r := result{out: o.String(), err: e.String()}
	if ee, ok := err.(*exec.ExitError); ok {
		r.code = ee.ExitCode()
		if r.code < 0 {
			t.Fatalf("%q killed by the signal: no handler", args)
		}
	}
	return r
}

// TestSignals: SIGINT and SIGTERM end streams with 0 and their summary,
// transmissions with their partial result, and commands without a result
// with 130 (docs/exitcodes.md).
func TestSignals(t *testing.T) {
	dir := t.TempDir()
	gap := filepath.Join(dir, "gap.log")
	_ = os.WriteFile(gap, []byte("(1700000000.000000) can0 100#11\n(1700000100.000000) can0 100#22\n"), 0o600)
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		want(t, signalled(t, sig, "monitor", "-i", "virtual", "--format", "json"), 0, `"type":"summary"`)
		want(t, signalled(t, sig, "record", "-i", "virtual", "-o", filepath.Join(dir, "r.pcap")), 0, "recorded 0 frames")
		want(t, signalled(t, sig, "simulate", "--dbc", vehicle, "--duration", "1m"), 0, "Engine")
		want(t, signalled(t, sig, "replay", "-i", "virtual", gap), 0, "replayed 1 of 2")
		want(t, signalled(t, sig, "send", "-i", "virtual", "--gap", "1m", "123#00", "123#01"), 0, "sent 1 frame(s)")
		want(t, signalled(t, sig, "isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "--format", "json"), 130, `"code": "canceled"`)
		want(t, signalled(t, sig, "isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "100", "--format", "json"), 130, `"code": "canceled"`)
	}
	// The recording is a complete pcap file.
	want(t, run(t, "", "trace", "info", filepath.Join(dir, "r.pcap")), 0, "0 frames")
}

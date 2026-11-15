// SPDX-License-Identifier: MIT

//go:build unix

package cmd

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/otfabric/go-canbus/virtual"
)

// sendVirtual sends frames to the shared virtual bus from a port of its
// own.
func sendVirtual(t *testing.T, frames ...string) {
	t.Helper()
	p, err := sharedVirtual().Open(virtual.WithFD())
	if err != nil {
		t.Error(err)
		return
	}
	for _, s := range frames {
		f, err := parseForTest(s)
		if err == nil {
			err = p.Send(context.Background(), &f)
		}
		if err != nil {
			t.Error(err)
		}
	}
}

// TestTraceOutputClosedPipe: a trace output whose reader has gone (a FIFO
// whose only reader closes once the command waits: every write fails with
// EPIPE) is an output failure, exit 10 output_failed. The trace writers
// buffer, so the failure comes with the final write, whose error monitor
// -o and xl monitor -o ignored (exit 0).
func TestTraceOutputClosedPipe(t *testing.T) {
	for _, c := range []struct {
		ready string
		args  []string
	}{
		{"monitor", []string{"monitor", "-i", "virtual", "-n", "1", "-o"}},
		{"xl monitor", []string{"xl", "monitor", "-i", "virtual", "-n", "1", "-o"}},
		{"record", []string{"record", "-i", "virtual", "-n", "1", "-o"}},
	} {
		for _, format := range []string{"json", "text"} {
			p := filepath.Join(t.TempDir(), "out.log")
			if err := syscall.Mkfifo(p, 0o600); err != nil {
				t.Skip("no FIFO:", err)
			}
			// A reader that does not block lets the command open the FIFO
			// for writing; it closes once the command is ready.
			reader, err := os.OpenFile(p, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if err != nil {
				t.Fatal(err)
			}
			onReady(t, c.ready, func() {
				_ = reader.Close()
				sendVirtual(t, "123#0102")
			})
			r := run(t, "", append(c.args, p, "--format", format)...)
			_ = reader.Close()
			if format == "json" {
				want(t, r, 10, `"code": "output_failed"`, "out.log")
			} else {
				want(t, r, 10, "write "+p)
			}
		}
	}
}

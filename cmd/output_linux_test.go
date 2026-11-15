// SPDX-License-Identifier: MIT

//go:build linux

package cmd

import (
	"os"
	"testing"
)

// TestTraceOutputFailure: a trace output whose writes fail (/dev/full:
// every write fails with ENOSPC) is an output failure, exit 10
// output_failed. The trace writers buffer: the failure comes with the
// final flush, whose error monitor -o and xl monitor -o ignored (exit 0).
func TestTraceOutputFailure(t *testing.T) {
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full")
	}
	for _, c := range []struct {
		ready string
		args  []string
	}{
		{"monitor", []string{"monitor", "-i", "virtual", "-o", "/dev/full", "-n", "1"}},
		{"xl monitor", []string{"xl", "monitor", "-i", "virtual", "-o", "/dev/full", "-n", "1"}},
		{"record", []string{"record", "-i", "virtual", "-o", "/dev/full", "-n", "1"}},
	} {
		// One hook per run: a hook fires once.
		onReady(t, c.ready, func() { sendVirtual(t, "123#0102") })
		r := run(t, "", append(c.args, "--format", "json")...)
		want(t, r, 10, `"code": "output_failed"`, "/dev/full")
		onReady(t, c.ready, func() { sendVirtual(t, "123#0102") })
		r = run(t, "", c.args...)
		want(t, r, 10, "write /dev/full")
	}
}

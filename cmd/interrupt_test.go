// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInterrupted: what each long-running command does when its context
// is cancelled (SIGINT/SIGTERM in Execute). Streams and transmissions
// with a partial result end with 0 and print it; a command without a
// result ends with 130 (canceled). isotp receive --count 1 ended with 5
// (timeout) although no --timeout was given.
func TestInterrupted(t *testing.T) {
	gap := writeFile(t, "gap.log", "(1700000000.000000) can0 100#11\n(1700000100.000000) can0 100#22\n")
	rec := filepath.Join(t.TempDir(), "r.log")
	for _, c := range []struct {
		args []string
		code int
		out  string
	}{
		{[]string{"monitor", "-i", "virtual"}, 0, ""},
		{[]string{"record", "-i", "virtual", "-o", rec}, 0, "recorded 0 frames"},
		{[]string{"stats", "-i", "virtual", "--duration", "1m"}, 0, "live"},
		{[]string{"xl", "monitor", "-i", "virtual"}, 0, ""},
		{[]string{"isotp", "monitor", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8"}, 0, ""},
		{[]string{"isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "0"}, 0, ""},
		{[]string{"isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "1", "--format", "json"}, 130, `"code": "canceled"`},
		{[]string{"isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "100"}, 130, "canceled"},
		{[]string{"send", "-i", "virtual", "--gap", "1m", "123#00", "123#01"}, 0, "sent 1 frame(s)"},
		{[]string{"replay", "-i", "virtual", gap}, 0, "replayed 1 of 2"},
		{[]string{"simulate", "--dbc", "testdata/vehicle.dbc", "--duration", "1m"}, 0, "Engine"},
		{[]string{"inject", "--frame", "123#00", "--delay-probability", "1", "--delay", "1m"}, 0, "1 delayed"},
	} {
		resetVirtual()
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(300*time.Millisecond, cancel)
		var out, errOut bytes.Buffer
		began := time.Now()
		code := Run(ctx, c.args, strings.NewReader(""), &out, &errOut)
		cancel()
		if el := time.Since(began); el > 5*time.Second {
			t.Fatalf("%q took %v after the interrupt", c.args, el)
		}
		want(t, result{code, out.String(), errOut.String()}, c.code, c.out)
	}
	// --timeout still ends isotp receive --count with 5.
	want(t, run(t, "", "isotp", "receive", "-i", "virtual", "--tx-id", "7E8", "--rx-id", "7E0", "-n", "1", "--timeout", "50ms"), 5, "received 0 of 1")
}

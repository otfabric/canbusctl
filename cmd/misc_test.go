// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
)

func openTraceForTest(p string) (capture.Reader, func(), error) { return cli.OpenTrace(p, os.Stdin) }

func jsonOf(t *testing.T, s string) cli.FrameJSON {
	f, err := cli.ParseFrame(s)
	if err != nil {
		t.Fatal(err)
	}
	return cli.ToJSON(&f)
}

func TestRootAndUsage(t *testing.T) {
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		want(t, run(t, "", "completion", sh), 0)
	}
	want(t, run(t, "", "completion", "tcsh"), 3)
	want(t, run(t, "", "nope"), 2, "unknown command", "--help")
	want(t, run(t, "", "nope", "--format", "json"), 2, `"kind": "usage"`)
	want(t, run(t, "", "version", "--format", "xml"), 3, "invalid --format")
	t.Setenv("CANBUSCTL_FORMAT", "json")
	want(t, run(t, "", "version"), 0, `{"name":"canbusctl",`)
	want(t, run(t, "", "send", "--bogus"), 2, `"usage"`)
	if !strings.Contains(run(t, "", "--help").out, "Passive by default") {
		t.Fatal("help")
	}
}

func TestInterfacesOffLinux(t *testing.T) {
	if linux() {
		t.Skip("covered by the vcan tests")
	}
	for _, args := range [][]string{{"interfaces"}, {"info", "vcan0"}, {"up", "vcan0", "--yes"}, {"down", "vcan0", "--yes"},
		{"configure", "vcan0", "--bitrate", "500000", "--yes"}, {"configure", "vcan0", "--mtu", "72", "--yes"},
		{"configure", "vcan0", "--restart", "--yes"}} {
		want(t, run(t, "", args...), 8, "Linux")
	}
}

func TestConfigureValidation(t *testing.T) {
	want(t, run(t, "", "configure", "vcan0"), 3, "nothing to configure")
	want(t, run(t, "", "configure", "vcan0", "--sample-point", "120"), 3, "percentage")
	// NaN and the infinities are not percentages: NaN passed the range
	// check and reached the --yes check (exit 13).
	for _, flag := range []string{"--sample-point", "--dsample-point"} {
		for _, v := range []string{"NaN", "nan", "+Inf", "Inf", "-Inf", "100", "-0.1", "100.1"} {
			want(t, run(t, "", "configure", "vcan0", flag, v), 3, flag[2:]+" ", "percentage between 0 and 100")
		}
		for _, v := range []string{"0.1", "87.5", "99.9"} {
			want(t, run(t, "", "configure", "vcan0", flag, v), 13)
		}
	}
	want(t, run(t, "", "configure", "vcan0", "--mtu", "50"), 3, "--mtu")
	r := run(t, "", "configure", "vcan0", "--fd", "--listen-only=false", "--dbitrate", "2000000", "--dsample-point", "75", "--restart-ms", "100", "--one-shot", "--berr-reporting", "--loopback")
	want(t, r, 13)
}

type canbusFrame = canbus.Frame

func parseForTest(s string) (canbus.Frame, error) { return cli.ParseFrame(s) }

// CANBUSCTL_OUTPUT_FORMAT is accepted; CANBUSCTL_FORMAT is canonical and wins.
func TestOutputFormatEnv(t *testing.T) {
	t.Setenv("CANBUSCTL_FORMAT", "")
	t.Setenv("CANBUSCTL_OUTPUT_FORMAT", "json")
	if r := run(t, "", "version"); !strings.Contains(r.out, `"name":"canbusctl"`) {
		t.Fatalf("OUTPUT_FORMAT ignored: %q", r.out)
	}
	t.Setenv("CANBUSCTL_FORMAT", "text")
	if r := run(t, "", "version"); strings.Contains(r.out, `"name":"canbusctl"`) {
		t.Fatalf("FORMAT does not win: %q", r.out)
	}
}

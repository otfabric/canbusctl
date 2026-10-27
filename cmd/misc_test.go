// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/otfabric/canbusctl/internal/cli"
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
	want(t, run(t, "", "version"), 0, "canbusctl dev", "go-canbus")
	SetBuildMeta("1.2.3", "v1.2.3", "abc", "today")
	want(t, run(t, "", "version", "--format", "json"), 0, `"version": "1.2.3"`, `"commit": "abc"`)
	SetBuildMeta("", "", "", "")
	for _, sh := range []string{"bash", "zsh", "fish", "powershell"} {
		want(t, run(t, "", "completion", sh), 0)
	}
	want(t, run(t, "", "completion", "tcsh"), 3)
	want(t, run(t, "", "nope"), 2, "unknown command", "--help")
	want(t, run(t, "", "nope", "--format", "json"), 2, `"kind": "usage"`)
	want(t, run(t, "", "version", "--format", "xml"), 3, "invalid --format")
	t.Setenv("CANBUSCTL_FORMAT", "json")
	want(t, run(t, "", "version"), 0, `"schemaVersion"`)
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
	want(t, run(t, "", "configure", "vcan0", "--mtu", "50"), 3, "--mtu")
	r := run(t, "", "configure", "vcan0", "--fd", "--listen-only=false", "--dbitrate", "2000000", "--dsample-point", "75", "--restart-ms", "100", "--one-shot", "--berr-reporting", "--loopback")
	want(t, r, 9)
}

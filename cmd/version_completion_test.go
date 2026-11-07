// SPDX-License-Identifier: MIT

package cmd

import (
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestVersionCommand(t *testing.T) {
	plat := runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH
	SetBuildMeta("0.10.0", "v0.10.0", "abc123", "2026-11-05T15:41:49Z")
	t.Cleanup(func() { SetBuildMeta("dev", "", "", "") })
	line := "canbusctl 0.10.0 (tag v0.10.0, commit abc123, built 2026-11-05T15:41:49Z, " + plat + ")\n"
	for _, args := range [][]string{{"version"}, {"version", "--format", "table"}, {"version", "--format", "text"}} {
		if r := run(t, "", args...); r.code != 0 || r.out != line {
			t.Fatalf("%q: exit %d, %q, want %q", args, r.code, r.out, line)
		}
	}
	js := `{"name":"canbusctl","version":"0.10.0","tag":"v0.10.0","commit":"abc123","built":"2026-11-05T15:41:49Z","go":"` +
		runtime.Version() + `","platform":"` + runtime.GOOS + "/" + runtime.GOARCH + "\"}\n"
	for _, args := range [][]string{{"version", "--json"}, {"version", "--format", "json"}, {"--format=json", "version"}} {
		if r := run(t, "", args...); r.code != 0 || r.out != js {
			t.Fatalf("%q: exit %d, %q, want %q", args, r.code, r.out, js)
		}
	}
	t.Setenv("CANBUSCTL_FORMAT", "json")
	if r := run(t, "", "version"); r.out != js {
		t.Fatalf("CANBUSCTL_FORMAT=json: %q", r.out)
	}
	t.Setenv("CANBUSCTL_FORMAT", "")
	SetBuildMeta("dev", "", "", "")
	if r := run(t, "", "version"); !strings.HasPrefix(r.out, "canbusctl dev (commit ") || strings.Contains(r.out, "tag ") ||
		strings.Count(r.out, "\n") != 1 {
		t.Fatalf("dev: %q", r.out)
	}
	want(t, run(t, "", "version", "extra"), 2)
}

// complete runs the hidden __complete command and returns the candidates
// (without descriptions) and the directive line.
func complete(t *testing.T, args ...string) ([]string, string) {
	t.Helper()
	r := run(t, "", append([]string{cobra.ShellCompRequestCmd}, args...)...)
	if r.code != 0 {
		t.Fatalf("__complete %q: exit %d\n%s", args, r.code, r.errOut)
	}
	var vals []string
	var directive string
	for _, l := range strings.Split(strings.TrimSpace(r.out), "\n") {
		if strings.HasPrefix(l, ":") {
			directive = l
			continue
		}
		vals = append(vals, strings.SplitN(l, "\t", 2)[0])
	}
	return vals, directive
}

func TestCompletionValues(t *testing.T) {
	noFile := ":4"  // ShellCompDirectiveNoFileComp
	fileExt := ":8" // ShellCompDirectiveFilterFileExt
	for _, tc := range []struct {
		args      []string
		values    []string
		directive string
	}{
		{[]string{"completion", ""}, []string{"bash", "zsh", "fish", "powershell"}, noFile},
		{[]string{"--format", ""}, []string{"text", "json", "table"}, noFile},
		{[]string{"version", "--format", ""}, []string{"text", "json", "table"}, noFile},
		{[]string{"--slcan-data-bitrate", ""}, []string{"0", "2000000", "5000000"}, noFile},
		{[]string{"configure", "can0", "--mtu", ""}, []string{"16", "72"}, noFile},
		{[]string{"send", "--dlc", ""}, []string{"0", "1", "2", "3", "4", "5", "6", "7", "8"}, noFile},
		{[]string{"record", "--file-format", ""}, []string{"candump", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}, noFile},
		{[]string{"record", "--output", ""}, []string{"log", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}, fileExt},
		{[]string{"monitor", "--dbc", ""}, []string{"dbc", "kcd", "arxml"}, fileExt},
		{[]string{"dbc", "convert", "--to", ""}, []string{"dbc", "kcd", "json"}, noFile},
		{[]string{"dbc", "inspect", ""}, []string{"dbc", "kcd", "arxml"}, fileExt},
		{[]string{"dbc", "generate", "x.dbc", "--output", ""}, []string{"go", "json"}, fileExt},
		{[]string{"isotp", "send", "--addressing", ""}, []string{"normal", "extended", "mixed"}, noFile},
		{[]string{"isotp", "receive", "--tx-dl", ""}, []string{"8", "12", "16", "20", "24", "32", "48", "64"}, noFile},
		{[]string{"trace", "convert", "--file-format", ""}, []string{"candump", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}, noFile},
		{[]string{"trace", "info", ""}, []string{"log", "txt", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}, fileExt},
		{[]string{"simulate", "--pattern", ""}, []string{"counter", "random", "initial"}, noFile},
		{[]string{"inject", "--error-frame", ""}, []string{"bus-off", "passive", "warning", "no-ack", "stuff", "overflow", "restarted"}, noFile},
		{[]string{"bridge", "--side-a", ""}, nil, noFile},
		{[]string{"xl", "monitor", "--output", ""}, []string{"log", "jsonl", "pcap", "pcapng"}, fileExt},
		{[]string{"version", ""}, nil, noFile},
	} {
		vals, dir := complete(t, tc.args...)
		if !slices.Equal(vals, tc.values) || dir != tc.directive {
			t.Errorf("__complete %q: %q %s, want %q %s", tc.args, vals, dir, tc.values, tc.directive)
		}
	}
	vals, _ := complete(t, "--slcan-bitrate", "")
	if len(vals) != 11 || vals[0] != "0" || !slices.Contains(vals, "500000") {
		t.Errorf("--slcan-bitrate: %q", vals)
	}
}

// valueSet recognises a usage that lists the accepted values: "a, b or c",
// "a|b|c", "one of ...".
var valueSet = regexp.MustCompile(`\w+(, \w+)+,? or \w+|\w+\|\w+|one of `)

// Every flag that takes a value has a completion function, and every leaf
// command completes its positional arguments: an enum flag without its
// values (or a free value that offers files) fails here.
func TestEveryFlagCompletes(t *testing.T) {
	root := newRoot(&app{})
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" || f.Name == "help" {
				return
			}
			fn, ok := c.GetFlagCompletionFunc(f.Name)
			if !ok {
				t.Errorf("%s --%s: no completion", c.CommandPath(), f.Name)
				return
			}
			if valueSet.MatchString(f.Usage) {
				if vals, _ := fn(c, nil, ""); len(vals) == 0 {
					t.Errorf("%s --%s lists values (%q) but completes none", c.CommandPath(), f.Name, f.Usage)
				}
			}
		})
		if !c.HasSubCommands() && c.ValidArgsFunction == nil && len(c.ValidArgs) == 0 && c.Name() != "help" {
			t.Errorf("%s: positional arguments do not complete", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

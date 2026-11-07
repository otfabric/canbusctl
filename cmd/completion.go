// SPDX-License-Identifier: MIT

package cmd

import (
	"slices"
	"strconv"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus/slcan"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// File extensions completed for file arguments and flags.
var (
	extDatabase   = []string{"dbc", "kcd", "arxml"}
	extTraceRead  = []string{"log", "txt", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}
	extTraceWrite = []string{"log", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}
	extXLTrace    = []string{"log", "jsonl", "pcap", "pcapng"}
)

// traceFormats are the values of --file-format.
var traceFormats = []string{"candump", "jsonl", "pcap", "pcapng", "asc", "blf", "trc"}

func fixed(values ...string) cobra.CompletionFunc {
	return cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp)
}

func files(exts ...string) cobra.CompletionFunc {
	return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return exts, cobra.ShellCompDirectiveFilterFileExt
	}
}

func noFiles(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func intStrings(vs []int) []string {
	slices.Sort(vs)
	out := make([]string, len(vs))
	for i, v := range vs {
		out[i] = strconv.Itoa(v)
	}
	return out
}

// flagCompletions are the value completions of flags, by flag name; a
// "<command path> <flag>" key overrides the name for one command.
func flagCompletions() map[string]cobra.CompletionFunc {
	bitrates := []int{0}
	for b := range slcan.Bitrates {
		bitrates = append(bitrates, b)
	}
	dataBitrates := []int{0}
	for b := range slcan.DataBitrates {
		dataBitrates = append(dataBitrates, b)
	}
	return map[string]cobra.CompletionFunc{
		"slcan-bitrate":      fixed(intStrings(bitrates)...),
		"slcan-data-bitrate": fixed(intStrings(dataBitrates)...),
		"mtu":                fixed("16", "72"),
		"dlc":                fixed("0", "1", "2", "3", "4", "5", "6", "7", "8"),
		"tx-dl":              fixed("8", "12", "16", "20", "24", "32", "48", "64"),
		"file-format":        fixed(traceFormats...),
		"addressing":         fixed("normal", "extended", "mixed"),
		"pattern":            fixed("counter", "random", "initial"),
		"error-frame":        fixed("bus-off", "passive", "warning", "no-ack", "stuff", "overflow", "restarted"),
		"to":                 fixed("dbc", "kcd", "json"),
		"dbc":                files(extDatabase...),
		"trace":              files(extTraceRead...),
		"output":             files(extTraceWrite...),

		"canbusctl xl monitor output":   files(extXLTrace...),
		"canbusctl dbc generate output": files("go", "json"),
		"canbusctl dbc convert output":  files(extDatabase[0], extDatabase[1], "json"),
	}
}

// argCompletions are the completions of positional arguments, by command
// path. Commands without an entry complete no files.
func argCompletions() map[string]cobra.CompletionFunc {
	db := firstArg(files(extDatabase...))
	return map[string]cobra.CompletionFunc{
		"canbusctl dbc inspect":   db,
		"canbusctl dbc validate":  db,
		"canbusctl dbc decode":    db,
		"canbusctl dbc encode":    db,
		"canbusctl dbc generate":  db,
		"canbusctl dbc convert":   db,
		"canbusctl replay":        firstArg(files(extTraceRead...)),
		"canbusctl trace info":    firstArg(files(extTraceRead...)),
		"canbusctl trace convert": traceConvertArgs,
	}
}

// firstArg completes f for the first positional argument only.
func firstArg(f cobra.CompletionFunc) cobra.CompletionFunc {
	return func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return noFiles(c, args, toComplete)
		}
		return f(c, args, toComplete)
	}
}

func traceConvertArgs(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return files(extTraceRead...)(c, args, toComplete)
	case 1:
		return files(extTraceWrite...)(c, args, toComplete)
	}
	return noFiles(c, args, toComplete)
}

// registerCompletions gives every flag that takes a value, and the
// positional arguments of every command, a completion: fixed values, files
// of a known type, or no files for free values (addresses, numbers).
func registerCompletions(root *cobra.Command) {
	byFlag, byArgs := flagCompletions(), argCompletions()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		c.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Value.Type() == "bool" {
				return
			}
			if _, ok := c.GetFlagCompletionFunc(f.Name); ok {
				return
			}
			fn, ok := byFlag[c.CommandPath()+" "+f.Name]
			if !ok {
				fn, ok = byFlag[f.Name]
			}
			if !ok {
				fn = noFiles
			}
			_ = c.RegisterFlagCompletionFunc(f.Name, fn)
		})
		if !c.HasSubCommands() && c.ValidArgsFunction == nil && len(c.ValidArgs) == 0 {
			if fn, ok := byArgs[c.CommandPath()]; ok {
				c.ValidArgsFunction = fn
			} else {
				c.ValidArgsFunction = noFiles
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion bash|zsh|fish|powershell",
		Short: "Generate a shell completion script",
		Long: `completion writes a completion script for bash, zsh, fish or PowerShell to
stdout. Commands, flags, the values of flags with a fixed set (--format,
--file-format, --addressing, --pattern, --to, --slcan-bitrate, ...) and
database and trace files by extension complete. Installation: README.md,
"Shell completion".`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			root, out := cmd.Root(), cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return errs.Invalid("unknown shell %q (bash, zsh, fish or powershell)", args[0])
		},
	}
}

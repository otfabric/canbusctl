// SPDX-License-Identifier: MIT

// Package cmd is the command tree of canbusctl.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/canbusctl/internal/output"
	"github.com/spf13/cobra"
)

var buildMeta struct{ version, tag, commit, buildDate string }

// SetBuildMeta records link-time metadata (main's -X flags).
func SetBuildMeta(version, tag, commit, buildDate string) {
	buildMeta.version, buildMeta.tag, buildMeta.commit, buildMeta.buildDate = version, tag, commit, buildDate
}

// app is the state of one invocation.
type app struct {
	in      io.Reader
	out     io.Writer
	errOut  io.Writer
	format  output.Format
	started bool // RunE was reached: flags parsed
}

// formatOf reads --format before anything else, so that usage errors can
// follow it too.
func formatOf(args []string) output.Format {
	v := os.Getenv("CANBUSCTL_FORMAT")
	for i, a := range args {
		switch {
		case a == "--format" && i+1 < len(args):
			v = args[i+1]
		case strings.HasPrefix(a, "--format="):
			v = strings.TrimPrefix(a, "--format=")
		}
	}
	f, err := output.Parse(v)
	if err != nil {
		return output.Text
	}
	return f
}

func newRoot(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "canbusctl",
		Short: "Capture, decode, diagnose, transmit, replay and simulate CAN and CAN FD traffic",
		Long: `canbusctl is a command-line toolkit for CAN, CAN FD, ISO-TP and DBC/KCD databases,
built on github.com/otfabric/go-canbus.

Passive by default: monitor, record, stats and the dbc commands never transmit.
Commands that transmit (send, send-fd, replay, isotp send, simulate, inject)
need --yes on any interface that is not virtual (vcan, vxcan or the in-process
bus "virtual"), and are bounded by --rate, --max-frames and --duration.

SocketCAN interfaces exist on Linux only; on macOS and Windows the database,
trace and virtual-bus commands work and interface commands exit 8.

Results go to stdout as text (default), a table or JSON (--format); progress
and diagnostics go to stderr. Exit codes: docs/exitcodes.md.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			f, err := output.Parse(cmd.Flag("format").Value.String())
			if err != nil {
				return errs.Invalid("%v", err)
			}
			a.format = f
			a.started = true
			return nil
		},
	}
	root.PersistentFlags().String("format", envOr("CANBUSCTL_FORMAT", "text"), "Output format: text, json or table (env CANBUSCTL_FORMAT)")
	_ = root.RegisterFlagCompletionFunc("format", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"text\thuman-readable", "json\tversioned JSON", "table\taligned columns"}, cobra.ShellCompDirectiveNoFileComp
	})
	root.AddCommand(
		newInterfacesCmd(a), newInfoCmd(a), newConfigureCmd(a), newUpDownCmd(a, true), newUpDownCmd(a, false),
		newMonitorCmd(a), newSendCmd(a, false), newSendCmd(a, true), newRecordCmd(a), newReplayCmd(a), newStatsCmd(a),
		newDBCCmd(a), newISOTPCmd(a), newSimulateCmd(a), newInjectCmd(a),
		newVersionCmd(a), newCompletionCmd(),
	)
	return root
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// emit writes a result in the selected format.
func (a *app) emit(command string, r output.Result) error {
	if err := output.Write(a.out, a.format, command, r); err != nil {
		return errs.New(errs.KindOutput, errs.CodeOutputFailed, "write output: %w", err)
	}
	return nil
}

// stream returns a record stream on stdout.
func (a *app) stream() *output.Stream { return output.NewStream(a.out, a.format) }

// warn writes a diagnostic line to stderr.
func (a *app) warn(format string, args ...any) {
	fmt.Fprintf(a.errOut, "canbusctl: "+format+"\n", args...)
}

// Run executes canbusctl with args and returns the exit code.
func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	a := &app{in: in, out: out, errOut: errOut, format: formatOf(args)}
	root := newRoot(a)
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return errs.ExitOK
	}
	if errors.Is(err, errs.ErrFindings) {
		return errs.ExitFindings
	}
	var e *errs.Error
	if !a.started || isUsage(err) {
		e = &errs.Error{Kind: errs.KindUsage, Code: errs.CodeInvalidInput, Message: err.Error(), Cause: err}
		var typed *errs.Error
		if errors.As(err, &typed) {
			e = typed
		}
	} else {
		e = errs.Classify(err)
	}
	if a.format == output.JSON {
		_ = output.WriteError(out, e)
	} else {
		fmt.Fprintf(errOut, "error: %s\n", e.Message)
		if e.Kind == errs.KindUsage {
			fmt.Fprintln(errOut, "Run 'canbusctl --help' for usage.")
		}
	}
	return errs.ExitCode(e.Kind)
}

// isUsage recognises cobra's own errors about the command line, which
// can also arrive after flags were parsed (required flags are checked
// late).
func isUsage(err error) bool {
	var typed *errs.Error
	if errors.As(err, &typed) {
		return false
	}
	msg := err.Error()
	for _, p := range []string{"required flag", "accepts ", "requires at least", "unknown command", "unknown flag",
		"unknown shorthand", "invalid argument", "flag needs an argument", "if any flags in the group"} {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// Execute runs canbusctl on the process's arguments, ending on SIGINT or
// SIGTERM.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

type versionResult struct {
	Version   string `json:"version"`
	Tag       string `json:"tag,omitempty"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"buildDate,omitempty"`
	GoCanbus  string `json:"goCanbus"`
	Go        string `json:"go"`
	Platform  string `json:"platform"`
}

func (v versionResult) Text() string {
	s := "canbusctl " + v.Version
	if v.Commit != "" {
		s += " (" + v.Commit + ")"
	}
	return s + "\ngo-canbus " + v.GoCanbus + "\n" + v.Go + " " + v.Platform
}

func newVersionCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version of canbusctl and go-canbus",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			v := versionResult{Version: buildMeta.version, Tag: buildMeta.tag, Commit: buildMeta.commit, BuildDate: buildMeta.buildDate,
				GoCanbus: "(unknown)", Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
			if v.Version == "" {
				v.Version = "dev"
			}
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, d := range info.Deps {
					if d.Path == "github.com/otfabric/go-canbus" {
						v.GoCanbus = d.Version
					}
				}
			}
			return a.emit("version", v)
		},
	}
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion bash|zsh|fish|powershell",
		Short:     "Generate a shell completion script",
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

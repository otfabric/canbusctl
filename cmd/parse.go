// SPDX-License-Identifier: MIT

package cmd

import (
	"strconv"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
	"github.com/spf13/pflag"
)

func captureLine(l string, f *canbus.Frame) error  { return capture.ParseCandumpLine(l, f) }
func captureFrame(s string, f *canbus.Frame) error { return capture.ParseFrame(s, f) }

// intFlag is the value of every int flag. pflag's own int flag parses 64
// bits and keeps the low 32 of them on 32-bit platforms; intFlag refuses
// a value an int cannot hold on this platform instead, as a usage error
// (exit 2), as pflag refuses a value beyond 64 bits.
type intFlag struct{ p *int }

func (f intFlag) Set(s string) error {
	v, err := strconv.ParseInt(s, 0, strconv.IntSize)
	if err != nil {
		return err
	}
	*f.p = int(v)
	return nil
}

func (f intFlag) Type() string   { return "int" }
func (f intFlag) String() string { return strconv.Itoa(*f.p) }

// intVar defines an int flag, as pflag's IntVarP, of type intFlag.
func intVar(fs *pflag.FlagSet, p *int, name, shorthand string, value int, usage string) {
	*p = value
	fs.VarP(intFlag{p}, name, shorthand, usage)
}

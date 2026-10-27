// SPDX-License-Identifier: MIT

package cmd

import (
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
)

func captureLine(l string, f *canbus.Frame) error  { return capture.ParseCandumpLine(l, f) }
func captureFrame(s string, f *canbus.Frame) error { return capture.ParseFrame(s, f) }

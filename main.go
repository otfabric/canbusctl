// SPDX-License-Identifier: MIT

// Command canbusctl captures, decodes, diagnoses, transmits, replays and
// simulates CAN and CAN FD traffic.
package main

import (
	"os"

	"github.com/otfabric/canbusctl/cmd"
)

// Injected at link time: -X main.version=... etc.
var (
	version   = "dev"
	tag       = ""
	commit    = ""
	buildDate = ""
)

func main() {
	cmd.SetBuildMeta(version, tag, commit, buildDate)
	os.Exit(cmd.Execute())
}

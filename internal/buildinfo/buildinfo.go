// SPDX-License-Identifier: MIT

// Package buildinfo resolves the build metadata of canbusctl (version, tag,
// commit, build date) and renders the output of the version command.
//
// The values are injected at link time into package main (-X main.version,
// main.tag, main.commit, main.buildDate) and handed over with [Resolve].
// Without them, the module version and VCS stamp of runtime/debug fill in.
package buildinfo

import (
	"bytes"
	"encoding/json"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
)

// Info is the resolved build metadata. Every field but Tag is non-empty.
type Info struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Tag      string `json:"tag"`
	Commit   string `json:"commit"`
	Built    string `json:"built"`
	Go       string `json:"go"`
	Platform string `json:"platform"`
}

// release matches the module version of `go install …@vX.Y.Z`; "(devel)",
// "+dirty" builds and pseudo-versions (a go build in a checkout) do not
// count as a release.
var (
	release = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)
	pseudo  = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)
)

// readBuildInfo is replaced in tests.
var readBuildInfo = debug.ReadBuildInfo

// Resolve fills in the metadata of name from the injected values, falling
// back to the binary's build information.
func Resolve(name, version, tag, commit, buildDate string) Info {
	i := Info{Name: name, Version: strings.TrimPrefix(version, "v"), Tag: tag, Commit: commit, Built: buildDate,
		Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if i.Version == "dev" {
		i.Version = ""
	}
	if bi, ok := readBuildInfo(); ok {
		if i.Version == "" && release.MatchString(bi.Main.Version) && !pseudo.MatchString(bi.Main.Version) {
			i.Version = strings.TrimPrefix(bi.Main.Version, "v")
		}
		if i.Commit == "" {
			var rev string
			var dirty bool
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					rev = s.Value
				case "vcs.modified":
					dirty = s.Value == "true"
				}
			}
			if rev != "" && dirty {
				rev += "-dirty"
			}
			i.Commit = rev
		}
	}
	if i.Version == "" {
		i.Version = "dev"
	}
	if i.Commit == "" {
		i.Commit = "unknown"
	}
	if i.Built == "" {
		i.Built = "unknown"
	}
	return i
}

// Text is the one-line text form:
//
//	<name> <version> (tag <tag>, commit <commit>, built <built>, <go>, <goos>/<goarch>)
//
// without the tag part when there is no tag.
func (i Info) Text() string {
	var b strings.Builder
	b.WriteString(i.Name + " " + i.Version + " (")
	if i.Tag != "" {
		b.WriteString("tag " + i.Tag + ", ")
	}
	b.WriteString("commit " + i.Commit + ", built " + i.Built + ", " + i.Go + ", " + i.Platform + ")")
	return b.String()
}

// JSON is the one-line JSON form, keys in the order name, version, tag,
// commit, built, go, platform.
func (i Info) JSON() string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(i) // a struct of strings always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

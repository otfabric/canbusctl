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
	"path"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
)

// Info is the resolved build metadata. Every field but Tag is non-empty;
// Libraries is never nil.
type Info struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Tag       string    `json:"tag"`
	Commit    string    `json:"commit"`
	Built     string    `json:"built"`
	Libraries []Library `json:"libraries"`
	Go        string    `json:"go"`
	Platform  string    `json:"platform"`
}

// Library is an OT Fabric library (github.com/otfabric/go-*) the binary is
// built on.
type Library struct {
	Name    string `json:"name"`    // last path element, e.g. go-canbus
	Version string `json:"version"` // without the leading v
}

const libraryPrefix = "github.com/otfabric/go-"

// libraries lists the github.com/otfabric/go-* dependencies of bi (their
// replacement where one is set), without the main module, sorted by name.
func libraries(bi *debug.BuildInfo) []Library {
	libs := []Library{}
	for _, d := range bi.Deps {
		if d.Replace != nil {
			d = d.Replace
		}
		if !strings.HasPrefix(d.Path, libraryPrefix) || d.Path == bi.Main.Path {
			continue
		}
		libs = append(libs, Library{Name: path.Base(d.Path), Version: strings.TrimPrefix(d.Version, "v")})
	}
	sort.Slice(libs, func(a, b int) bool { return libs[a].Name < libs[b].Name })
	return libs
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
		Libraries: []Library{}, Go: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH}
	if i.Version == "dev" {
		i.Version = ""
	}
	if bi, ok := readBuildInfo(); ok {
		i.Libraries = libraries(bi)
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
// without the tag part when there is no tag. The libraries come after the
// build date as "<name> <version>", none when there are none:
//
//	canbusctl 0.10.0 (tag v0.10.0, commit …, built …, go-canbus 0.8.0, go1.26.10, linux/amd64)
func (i Info) Text() string {
	var b strings.Builder
	b.WriteString(i.Name + " " + i.Version + " (")
	if i.Tag != "" {
		b.WriteString("tag " + i.Tag + ", ")
	}
	b.WriteString("commit " + i.Commit + ", built " + i.Built + ", ")
	for _, l := range i.Libraries {
		b.WriteString(l.Name + " " + l.Version + ", ")
	}
	b.WriteString(i.Go + ", " + i.Platform + ")")
	return b.String()
}

// JSON is the one-line JSON form, keys in the order name, version, tag,
// commit, built, libraries, go, platform; libraries is [] when there are none.
func (i Info) JSON() string {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if i.Libraries == nil {
		i.Libraries = []Library{}
	}
	_ = e.Encode(i) // a struct of strings always encodes
	return strings.TrimSuffix(b.String(), "\n")
}

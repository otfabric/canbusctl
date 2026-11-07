// SPDX-License-Identifier: MIT

package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"
)

const sha = "1802d0f9afe8642d1e278120b76fcdfac967cb9e"

var goPlat = runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH

func withBuildInfo(t *testing.T, bi *debug.BuildInfo) {
	t.Helper()
	old := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) { return bi, bi != nil }
	t.Cleanup(func() { readBuildInfo = old })
}

func TestTextAllFields(t *testing.T) {
	withBuildInfo(t, nil)
	i := Resolve("canbusctl", "0.10.0", "v0.10.0", sha, "2026-11-05T15:41:49Z")
	want := "canbusctl 0.10.0 (tag v0.10.0, commit " + sha + ", built 2026-11-05T15:41:49Z, " + goPlat + ")"
	if got := i.Text(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestTextWithoutTag(t *testing.T) {
	withBuildInfo(t, nil)
	i := Resolve("canbusctl", "v0.10.0", "", sha, "2026-11-05T15:41:49Z")
	want := "canbusctl 0.10.0 (commit " + sha + ", built 2026-11-05T15:41:49Z, " + goPlat + ")"
	if got := i.Text(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestDevFallback(t *testing.T) {
	withBuildInfo(t, nil)
	want := "canbusctl dev (commit unknown, built unknown, " + goPlat + ")"
	for _, v := range []string{"", "dev"} {
		if got := Resolve("canbusctl", v, "", "", "").Text(); got != want {
			t.Fatalf("got  %q\nwant %q", got, want)
		}
	}
}

func TestBuildInfoFallback(t *testing.T) {
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "(devel)"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "true"}}})
	i := Resolve("canbusctl", "", "", "", "")
	if i.Version != "dev" || i.Commit != sha+"-dirty" || i.Built != "unknown" {
		t.Fatalf("%+v", i)
	}
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v0.10.0"},
		Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}}})
	i = Resolve("canbusctl", "", "", "", "")
	if i.Version != "0.10.0" || i.Commit != sha || i.Tag != "" {
		t.Fatalf("%+v", i)
	}
	for _, v := range []string{"(devel)", "", "v0.9.1-0.20261107212138-d118346861a6+dirty", "v0.9.1-0.20261107212138-d118346861a6"} {
		withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: v}})
		if i := Resolve("canbusctl", "", "", "", ""); i.Version != "dev" || i.Commit != "unknown" {
			t.Fatalf("%q: %+v", v, i)
		}
	}
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Version: "v1.0.0-rc.1"}})
	if i := Resolve("canbusctl", "", "", "", ""); i.Version != "1.0.0-rc.1" {
		t.Fatalf("%+v", i)
	}
	// Injected values win over the build information.
	i = Resolve("canbusctl", "1.2.3", "v1.2.3", "abc", "today")
	if i.Version != "1.2.3" || i.Commit != "abc" {
		t.Fatalf("%+v", i)
	}
}

func TestJSON(t *testing.T) {
	withBuildInfo(t, nil)
	plat := runtime.GOOS + "/" + runtime.GOARCH
	i := Resolve("canbusctl", "0.10.0", "v0.10.0", sha, "2026-11-05T15:41:49Z")
	want := `{"name":"canbusctl","version":"0.10.0","tag":"v0.10.0","commit":"` + sha +
		`","built":"2026-11-05T15:41:49Z","libraries":[],"go":"` + runtime.Version() + `","platform":"` + plat + `"}`
	if got := i.JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	want = `{"name":"canbusctl","version":"dev","tag":"","commit":"unknown","built":"unknown","libraries":[],"go":"` +
		runtime.Version() + `","platform":"` + plat + `"}`
	if got := Resolve("canbusctl", "", "", "", "").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestLibrariesFormatting(t *testing.T) {
	plat := runtime.GOOS + "/" + runtime.GOARCH
	base := Info{Name: "canopenctl", Version: "0.6.0", Tag: "v0.6.0", Commit: sha, Built: "2026-11-05T15:41:49Z",
		Go: runtime.Version(), Platform: plat}
	head := "canopenctl 0.6.0 (tag v0.6.0, commit " + sha + ", built 2026-11-05T15:41:49Z, "
	jhead := `{"name":"canopenctl","version":"0.6.0","tag":"v0.6.0","commit":"` + sha + `","built":"2026-11-05T15:41:49Z","libraries":`
	jtail := `,"go":"` + runtime.Version() + `","platform":"` + plat + `"}`
	for _, tc := range []struct {
		libs       []Library
		text, json string
	}{
		{nil, "", `[]`},
		{[]Library{}, "", `[]`},
		{[]Library{{"go-canbus", "0.8.0"}}, "go-canbus 0.8.0, ", `[{"name":"go-canbus","version":"0.8.0"}]`},
		{[]Library{{"go-canbus", "0.8.0"}, {"go-canopen", "0.5.1"}}, "go-canbus 0.8.0, go-canopen 0.5.1, ",
			`[{"name":"go-canbus","version":"0.8.0"},{"name":"go-canopen","version":"0.5.1"}]`},
	} {
		i := base
		i.Libraries = tc.libs
		if got, want := i.Text(), head+tc.text+goPlat+")"; got != want {
			t.Errorf("text\ngot  %q\nwant %q", got, want)
		}
		if got, want := i.JSON(), jhead+tc.json+jtail; got != want {
			t.Errorf("json\ngot  %s\nwant %s", got, want)
		}
	}
}

func TestLibrariesFromBuildInfo(t *testing.T) {
	withBuildInfo(t, &debug.BuildInfo{Main: debug.Module{Path: "github.com/otfabric/go-self", Version: "(devel)"},
		Deps: []*debug.Module{
			{Path: "github.com/spf13/cobra", Version: "v1.10.2"},
			{Path: "github.com/otfabric/go-canopen", Version: "v0.5.1"},
			{Path: "github.com/otfabric/go-self", Version: "v9.9.9"},
			{Path: "github.com/otfabric/go-canbus", Version: "v0.8.0",
				Replace: &debug.Module{Path: "github.com/otfabric/go-canbus", Version: "v0.8.1-0.20261107212138-d118346861a6"}},
			{Path: "github.com/otfabric/docsync", Version: "v0.3.1"},
		}})
	i := Resolve("x", "1.0.0", "", "c", "b")
	want := []Library{{"go-canbus", "0.8.1-0.20261107212138-d118346861a6"}, {"go-canopen", "0.5.1"}}
	if len(i.Libraries) != len(want) || i.Libraries[0] != want[0] || i.Libraries[1] != want[1] {
		t.Fatalf("%+v", i.Libraries)
	}
	withBuildInfo(t, &debug.BuildInfo{})
	if i := Resolve("x", "", "", "", ""); i.Libraries == nil || len(i.Libraries) != 0 {
		t.Fatalf("%#v", i.Libraries)
	}
}

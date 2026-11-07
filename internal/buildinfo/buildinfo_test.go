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
		`","built":"2026-11-05T15:41:49Z","go":"` + runtime.Version() + `","platform":"` + plat + `"}`
	if got := i.JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	want = `{"name":"canbusctl","version":"dev","tag":"","commit":"unknown","built":"unknown","go":"` +
		runtime.Version() + `","platform":"` + plat + `"}`
	if got := Resolve("canbusctl", "", "", "", "").JSON(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

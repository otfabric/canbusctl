// SPDX-License-Identifier: MIT

//go:build interop

package interop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
)

// rawFixtures are the frame fixtures and the features a stack needs.
var rawFixtures = []struct {
	name  string
	needs []string
}{
	{"standard", nil},
	{"extended", nil},
	{"remote", []string{"remote"}},
	{"fd", []string{"fd", "brs"}},
	{"fd-esi", []string{"fd"}},
}

func missing(c *capabilities, feats []string) string {
	for _, f := range feats {
		if !c.Features[f] {
			return f
		}
	}
	return ""
}

func notations(fs []canbus.Frame) []string {
	var out []string
	for i := range fs {
		out = append(out, fs[i].String())
	}
	return out
}

func compareFrames(t *testing.T, got, want []canbus.Frame) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d frames, want %d", len(got), len(want))
	}
	for i := range want {
		if !sameFrame(&got[i], &want[i]) {
			t.Fatalf("frame %d: %v, want %v", i, got[i], want[i])
		}
	}
}

// TestCtlSendsStackReceives: canbusctl send → each stack's receiver.
func TestCtlSendsStackReceives(t *testing.T) {
	ifc := needVCAN(t)
	for _, a := range rawAdapters {
		for _, fx := range rawFixtures {
			scenario(t, a, "canbusctl send "+fx.name, func(t *testing.T) {
				c := caps(t, a)
				if !c.has("receive") {
					skipf(t, "%s has no receive", a)
				}
				if m := missing(c, fx.needs); m != "" {
					skipf(t, "%s lacks %s", a, m)
				}
				want := frames(t, fixture(t, "frames/"+fx.name+".jsonl"))
				args := []string{"receive", "--interface", ifc, "--count", strconv.Itoa(len(want)), "--timeout-ms", "30000"}
				if fx.name == "fd" || fx.name == "fd-esi" {
					args = append(args, "--fd")
				}
				p := start(t, a, args...)
				cmd := "send"
				if fx.name == "fd" || fx.name == "fd-esi" {
					cmd = "send-fd"
				}
				out, code := ctl(t, nil, append([]string{cmd, "-i", ifc, "--gap", "1ms"}, notations(want)...)...)
				if code != 0 {
					t.Fatalf("canbusctl send: exit %d: %s", code, out)
				}
				evs, code := p.wait(t, 60*time.Second)
				var got []canbus.Frame
				for _, e := range evs {
					if e.Type == "frame" && e.Frame != nil {
						got = append(got, e.Frame.frame(t))
					}
				}
				if code != 0 {
					t.Fatalf("%s receive: exit %d, %d frames", a, code, len(got))
				}
				compareFrames(t, got, want)
			})
		}
	}
}

// TestStackSendsCtlMonitors: each stack's sender → canbusctl monitor
// --format json.
func TestStackSendsCtlMonitors(t *testing.T) {
	ifc := needVCAN(t)
	for _, a := range rawAdapters {
		for _, fx := range rawFixtures {
			scenario(t, a, fx.name+" to canbusctl monitor", func(t *testing.T) {
				c := caps(t, a)
				if !c.has("send") {
					skipf(t, "%s has no send", a)
				}
				if m := missing(c, fx.needs); m != "" {
					skipf(t, "%s lacks %s", a, m)
				}
				src := fixture(t, "frames/"+fx.name+".jsonl")
				want := frames(t, src)
				mon := ctlStart(t, "monitor", "-i", ifc, "-n", strconv.Itoa(len(want)), "--duration", "60s", "--format", "json")
				args := []string{"send", "--interface", ifc, "--frames", "-"}
				if fx.name == "fd" || fx.name == "fd-esi" {
					args = append(args, "--fd")
				}
				if out, code := container(t, a, src, "", args...); code != 0 {
					t.Fatalf("%s send: exit %d: %s", a, code, out)
				}
				out, code := mon.wait(t, 70*time.Second)
				if code != 0 {
					t.Fatalf("monitor: exit %d: %s", code, out)
				}
				var got []canbus.Frame
				for _, r := range jsonRecords(t, out) {
					if str(r["type"]) != "frame" {
						continue
					}
					b, _ := json.Marshal(r)
					var d frameDoc
					_ = json.Unmarshal(b, &d)
					got = append(got, d.frame(t))
				}
				compareFrames(t, got, want)
			})
		}
	}
}

// TestRecordForPythonCan: canbusctl record writes .asc and .blf from a
// stack's traffic; python-can reads them back (log-convert, contract 1.1).
func TestRecordForPythonCan(t *testing.T) {
	ifc := needVCAN(t)
	for _, ext := range []string{".asc", ".blf", ".log"} {
		scenario(t, "pythoncan", "canbusctl record "+ext+", python-can reads", func(t *testing.T) {
			c := caps(t, "pythoncan")
			if !c.has("log-convert") {
				skipf(t, "pythoncan has no log-convert (contract 1.1)")
			}
			var want []canbus.Frame
			for _, name := range []string{"standard", "extended"} {
				want = append(want, frames(t, fixture(t, "frames/"+name+".jsonl"))...)
			}
			work := workDir(t)
			rec := ctlStart(t, "record", "-i", ifc, "-o", filepath.Join(work, "ctl"+ext), "-n", strconv.Itoa(len(want)), "--duration", "60s")
			if out, code := container(t, "canutils", docsJSONL(want), "", "send", "--interface", ifc, "--frames", "-"); code != 0 {
				t.Fatalf("canutils send: exit %d: %s", code, out)
			}
			if out, code := rec.wait(t, 70*time.Second); code != 0 {
				t.Fatalf("record: exit %d: %s", code, out)
			}
			_ = os.Chmod(filepath.Join(work, "ctl"+ext), 0o666)
			out, code := container(t, "pythoncan", nil, work, "log-convert", "--input", "/work/ctl"+ext, "--output", "/work/back.jsonl")
			if code != 0 {
				t.Fatalf("log-convert: exit %d: %s", code, out)
			}
			back, err := os.ReadFile(filepath.Join(work, "back.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			compareFrames(t, frames(t, back), want)
		})
	}
}

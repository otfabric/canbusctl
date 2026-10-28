// SPDX-License-Identifier: MIT

//go:build interop

package interop

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"
)

var dbcFixtures = []struct {
	name  string
	needs string
}{
	{"basic", ""}, {"wide", ""}, {"attributes", ""}, {"float", "dbcFloat"},
	{"mux", "dbcMultiplexing"}, {"extmux", "dbcExtendedMultiplexing"}, {"fd", "dbcFD"},
}

var arxmlFixtures = []string{"basic", "fd", "pdu-offset"}

// knownDifference is a disagreement between go-canbus (canbusctl) and an
// oracle that is a deliberate choice, recorded as "known-difference".
type knownDifference struct {
	adapter, fixture, contains, reason string
}

var knownDifferences = []knownDifference{
	{"cantools", "pdu-offset", "", "cantools places a PDU mapped at a byte offset of its frame at offset 0; go-canbus applies START-POSITION (AUTOSAR)"},
	{"canmatrix", "pdu-offset", "", "canmatrix places a PDU mapped at a byte offset of its frame at offset 0; go-canbus applies START-POSITION (AUTOSAR)"},
	{"canmatrix", "extmux", "EncodingComplexMultiplexed", "canmatrix decodes extended multiplexing but cannot encode it"},
	{"cantools", "", "but got 3", "cantools rejects a multiplexer value that selects no multiplexed signal; go-canbus decodes the switch and the always-present signals"},
	{"canmatrix", "", "payload longer than the message", "canmatrix rejects payloads longer than the message; go-canbus decodes the first Length bytes"},
}

func report(t *testing.T, adapter, fixture string, diffs []string) {
	t.Helper()
	for _, d := range diffs {
		known := false
		for _, k := range knownDifferences {
			if k.adapter == adapter && (k.fixture == "" || k.fixture == fixture) && strings.Contains(d, k.contains) {
				t.Logf("KNOWN DIFFERENCE: %s (%s)", d, k.reason)
				record(t, adapter, fixture+": "+d, "known-difference", k.reason)
				known = true
				break
			}
		}
		if !known {
			t.Errorf("%s", d)
		}
	}
}

type stackSignal struct {
	Raw      string          `json:"raw"`
	Physical json.RawMessage `json:"physical"`
	Label    string          `json:"label"`
}

type stackDoc struct {
	Type    string                 `json:"type"`
	Message string                 `json:"message"`
	Signals map[string]stackSignal `json:"signals"`
	Error   string                 `json:"error"`
	Data    string                 `json:"data"`
}

func parseStack(t *testing.T, out []byte) []stackDoc {
	t.Helper()
	var docs []stackDoc
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var d stackDoc
		if err := json.Unmarshal(line, &d); err != nil {
			t.Fatalf("bad line %q", line)
		}
		docs = append(docs, d)
	}
	return docs
}

func physical(raw json.RawMessage) float64 {
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	switch str(raw) {
	case "nan", "NaN":
		return math.NaN()
	case "inf", "+Inf":
		return math.Inf(1)
	case "-inf", "-Inf":
		return math.Inf(-1)
	}
	return math.NaN()
}

func samePhysical(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) && math.IsNaN(b)
	}
	return a == b || math.Abs(a-b) <= 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

// ctlDecoded is one frame of canbusctl dbc decode --format json.
type ctlDecoded struct {
	Message     string `json:"message"`
	DecodeError string `json:"decodeError"`
	Signals     []struct {
		Name     string          `json:"name"`
		Raw      string          `json:"raw"`
		Physical json.RawMessage `json:"physical"`
		Label    string          `json:"label"`
	} `json:"signals"`
}

func differ(c ctlDecoded, s stackDoc) string {
	switch {
	case s.Type == "decode-error" && (c.DecodeError != "" || c.Message == ""):
		return "" // both fail, or the identifier is in neither database
	case c.Message == "" && c.DecodeError == "":
		return "unknown identifier to canbusctl, the stack decodes " + s.Message
	case s.Type == "decode-error":
		if c.Message != "" && strings.Contains(strings.ToLower(s.Error), "length") {
			return "the stack rejects a payload longer than the message: " + s.Error
		}
		return "the stack fails (" + s.Error + "), canbusctl decodes"
	case c.DecodeError != "":
		return "canbusctl fails (" + c.DecodeError + "), the stack decodes"
	}
	if c.Message != s.Message {
		return fmt.Sprintf("message %q vs %q", c.Message, s.Message)
	}
	var diffs []string
	seen := map[string]bool{}
	for _, sig := range c.Signals {
		seen[sig.Name] = true
		ss, ok := s.Signals[sig.Name]
		switch {
		case !ok:
			diffs = append(diffs, sig.Name+" active only in canbusctl")
			continue
		case sig.Raw != ss.Raw:
			diffs = append(diffs, fmt.Sprintf("%s raw %s vs %s", sig.Name, sig.Raw, ss.Raw))
		case !samePhysical(physical(sig.Physical), physical(ss.Physical)):
			diffs = append(diffs, fmt.Sprintf("%s physical %s vs %s", sig.Name, sig.Physical, ss.Physical))
		case sig.Label != ss.Label:
			diffs = append(diffs, fmt.Sprintf("%s label %q vs %q", sig.Name, sig.Label, ss.Label))
		}
	}
	for name := range s.Signals {
		if !seen[name] {
			diffs = append(diffs, name+" active only in the stack")
		}
	}
	sort.Strings(diffs)
	return strings.Join(diffs, "; ")
}

// decodeDiff decodes fixtures/<kind>/<name>.frames.jsonl with canbusctl
// and the oracle and compares message, raw, physical and label values.
func decodeDiff(t *testing.T, a, kind, name string) {
	t.Helper()
	dir := t.TempDir()
	db := fixtureFile(t, dir, kind+"/"+name+"."+kind)
	src := fixture(t, kind+"/"+name+".frames.jsonl")
	in := frames(t, src)
	out, code := ctl(t, nil, append([]string{"dbc", "decode", db, "--format", "json"}, notations(in)...)...)
	if code != 0 && code != 1 {
		t.Fatalf("canbusctl decode: exit %d: %s", code, out)
	}
	recs := jsonRecords(t, out)
	sout, scode := container(t, a, src, "", "decode", "--db", "/fixtures/"+kind+"/"+name+"."+kind, "--frames", "-")
	if scode != 0 {
		t.Fatalf("%s decode: exit %d: %s", a, scode, sout)
	}
	docs := parseStack(t, sout)
	if len(recs) != len(in) || len(docs) != len(in) {
		t.Fatalf("%d canbusctl and %d %s results for %d frames", len(recs), len(docs), a, len(in))
	}
	var diffs []string
	for i := range in {
		b, _ := json.Marshal(recs[i])
		var c ctlDecoded
		_ = json.Unmarshal(b, &c)
		if d := differ(c, docs[i]); d != "" {
			diffs = append(diffs, fmt.Sprintf("frame %d (%s): %s", i, in[i].String(), d))
		}
	}
	report(t, a, name, diffs)
}

func TestDBCDecode(t *testing.T) {
	for _, a := range dbAdapters {
		for _, fx := range dbcFixtures {
			scenario(t, a, "decode "+fx.name, func(t *testing.T) {
				c := caps(t, a)
				if fx.needs != "" && !c.Features[fx.needs] {
					skipf(t, "%s lacks %s", a, fx.needs)
				}
				decodeDiff(t, a, "dbc", fx.name)
			})
		}
		for _, name := range arxmlFixtures {
			scenario(t, a, "decode arxml "+name, func(t *testing.T) {
				if !caps(t, a).Features["arxml"] {
					skipf(t, "%s lacks arxml (contract 1.1)", a)
				}
				decodeDiff(t, a, "arxml", name)
			})
		}
	}
}

// TestDBCEncode encodes each fixture message's physical values with
// canbusctl dbc encode and the oracle, and compares the bytes.
func TestDBCEncode(t *testing.T) {
	for _, a := range dbAdapters {
		for _, fx := range dbcFixtures {
			scenario(t, a, "encode "+fx.name, func(t *testing.T) {
				c := caps(t, a)
				if fx.needs != "" && !c.Features[fx.needs] {
					skipf(t, "%s lacks %s", a, fx.needs)
				}
				dir := t.TempDir()
				db := fixtureFile(t, dir, "dbc/"+fx.name+".dbc")
				src := fixture(t, "dbc/"+fx.name+".messages.jsonl")
				sout, scode := container(t, a, src, "", "encode", "--db", "/fixtures/dbc/"+fx.name+".dbc", "--messages", "-")
				if scode != 0 {
					t.Fatalf("%s encode: exit %d: %s", a, scode, sout)
				}
				docs := parseStack(t, sout)
				var diffs []string
				i := 0
				for _, line := range bytes.Split(src, []byte("\n")) {
					line = bytes.TrimSpace(line)
					if len(line) == 0 || line[0] == '#' {
						continue
					}
					var in struct {
						Message string                 `json:"message"`
						Signals map[string]json.Number `json:"signals"`
					}
					dec := json.NewDecoder(bytes.NewReader(line))
					dec.UseNumber()
					if err := dec.Decode(&in); err != nil {
						t.Fatal(err)
					}
					if i >= len(docs) {
						t.Fatalf("%d %s results", len(docs), a)
					}
					args := []string{"dbc", "encode", db, in.Message, "--lax", "--format", "json"}
					for k, v := range in.Signals {
						args = append(args, k+"="+v.String())
					}
					out, code := ctl(t, nil, args...)
					s := docs[i]
					i++
					var res struct {
						Result struct {
							Frame struct {
								Data string `json:"data"`
							} `json:"frame"`
						} `json:"result"`
					}
					_ = json.Unmarshal([]byte(out), &res)
					switch {
					case s.Type == "encode-error" && code != 0:
					case s.Type == "encode-error":
						diffs = append(diffs, fmt.Sprintf("%s: the stack fails (%s), canbusctl encodes %s", in.Message, s.Error, res.Result.Frame.Data))
					case code != 0:
						diffs = append(diffs, fmt.Sprintf("%s: canbusctl fails (exit %d), the stack encodes %s", in.Message, code, s.Data))
					default:
						got, _ := hex.DecodeString(res.Result.Frame.Data)
						exp, _ := hex.DecodeString(s.Data)
						n := min(len(got), len(exp))
						if !bytes.Equal(got[:n], exp[:n]) || bytes.Count(got[n:], []byte{0}) != len(got)-n || bytes.Count(exp[n:], []byte{0}) != len(exp)-n {
							diffs = append(diffs, fmt.Sprintf("%s: bytes %x vs %s", in.Message, got, s.Data))
						}
					}
				}
				report(t, a, fx.name, diffs)
			})
		}
	}
}

// TestKCDRoundTrip: canbusctl converts a DBC fixture to KCD; canmatrix
// converts that KCD back to DBC; both databases decode the fixture frames
// alike in canbusctl.
func TestKCDRoundTrip(t *testing.T) {
	scenario(t, "canmatrix", "canbusctl dbc→kcd, canmatrix kcd→dbc", func(t *testing.T) {
		if !caps(t, "canmatrix").has("convert") {
			skipf(t, "canmatrix has no convert")
		}
		work := workDir(t)
		db := fixtureFile(t, work, "dbc/basic.dbc")
		if out, code := ctl(t, nil, "dbc", "convert", db, "--to", "kcd", "-o", work+"/ctl.kcd"); code != 0 {
			t.Fatalf("canbusctl convert: exit %d: %s", code, out)
		}
		if out, code := container(t, "canmatrix", nil, work, "convert", "--input", "/work/ctl.kcd", "--output", "/work/back.dbc"); code != 0 {
			t.Fatalf("canmatrix convert: exit %d: %s", code, out)
		}
		in := frames(t, fixture(t, "dbc/basic.frames.jsonl"))
		a, _ := ctl(t, nil, append([]string{"dbc", "decode", db, "--format", "json"}, notations(in)...)...)
		b, _ := ctl(t, nil, append([]string{"dbc", "decode", work + "/back.dbc", "--format", "json"}, notations(in)...)...)
		ra, rb := jsonRecords(t, a), jsonRecords(t, b)
		if len(ra) != len(in) || len(rb) != len(in) {
			t.Fatalf("%d and %d results for %d frames", len(ra), len(rb), len(in))
		}
		var diffs []string
		for i := range ra {
			if string(ra[i]["signals"]) != string(rb[i]["signals"]) {
				diffs = append(diffs, fmt.Sprintf("frame %d: %s vs %s", i, ra[i]["signals"], rb[i]["signals"]))
			}
		}
		report(t, "canmatrix", "basic-kcd", diffs)
	})
}

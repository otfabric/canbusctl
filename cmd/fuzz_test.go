// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/token"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/chaos"
)

// documentedExit is the set of exit codes of docs/exitcodes.md.
var documentedExit = map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 8: true, 9: true, 10: true, 13: true, 130: true}

// runQuick runs canbusctl in process under a short deadline: fuzz
// targets must not wait.
func runQuick(stdin []byte, d time.Duration, args ...string) result {
	resetVirtual()
	var out, errOut bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	code := Run(ctx, args, bytes.NewReader(stdin), &out, &errOut)
	return result{code, out.String(), errOut.String()}
}

// checkJSONOutput asserts that output written with --format json is a
// sequence of JSON documents, an error document carries a known kind and
// code, and nothing is classified as an unexpected internal error.
func checkJSONOutput(t *testing.T, args []string, r result) {
	t.Helper()
	if !documentedExit[r.code] {
		t.Fatalf("%q: exit %d is not documented\n%s%s", args, r.code, r.out, r.errOut)
	}
	dec := json.NewDecoder(strings.NewReader(r.out))
	for {
		var m map[string]any
		err := dec.Decode(&m)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Fatalf("%q: output is not JSON: %v\n%s", args, err, r.out)
		}
		if e, ok := m["error"].(map[string]any); ok {
			if e["code"] == errs.CodeInternalUnexpected {
				t.Fatalf("%q: an unexpected error: %v", args, e["message"])
			}
			if errs.ExitCode(errs.Kind(e["kind"].(string))) != r.code {
				t.Fatalf("%q: kind %v but exit %d", args, e["kind"], r.code)
			}
		}
	}
}

func FuzzParseHexData(f *testing.F) {
	for _, s := range []string{"DEADBEEF", "de.ad.be.ef", "11 22:33", "", "1", "zz", "+1", "-1", "0x11"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, err := parseHexData(s)
		if err != nil {
			return
		}
		digits := strings.NewReplacer(".", "", " ", "", ":", "").Replace(s)
		if !strings.EqualFold(hex.EncodeToString(b), digits) {
			t.Fatalf("%q parsed as % X", s, b)
		}
	})
}

// FuzzFaultConfig: probabilities that bridge and inject accept are
// probabilities, and go-canbus takes them.
func FuzzFaultConfig(f *testing.F) {
	f.Add(0.1, 0.2, 0.3, 0.4, 0.5, int64(1), int64(time.Millisecond))
	f.Add(math.NaN(), 0.0, 0.0, 0.0, 0.0, int64(0), int64(-1))
	f.Add(1.0, 1.0, 1.0, 1.0, 1.0, int64(-7), int64(0))
	f.Fuzz(func(t *testing.T, drop, dup, delayP, reorder, corrupt float64, seed, delay int64) {
		ff := faultFlags{drop: drop, dup: dup, delayP: delayP, reorder: reorder, corrupt: corrupt, seed: seed, delay: time.Duration(delay)}
		cfg, err := ff.config(nil)
		if err != nil {
			return
		}
		for _, p := range []float64{cfg.Drop, cfg.Duplicate, cfg.Delay, cfg.Reorder, cfg.Corrupt} {
			if !(p >= 0 && p <= 1) {
				t.Fatalf("probability %v accepted", p)
			}
		}
		if _, err := chaos.New(cfg); err != nil {
			t.Fatalf("accepted, refused by go-canbus: %v", err)
		}
	})
}

// FuzzXLNotation: a CAN XL frame xl send accepts prints (as in its
// result) in a notation that parses back to the same frame.
func FuzzXLNotation(f *testing.F) {
	for _, s := range []string{"00242#80:03:00C0FFEE#11223344", "123#81:00:00000000#00", "7FF#83:FF:FFFFFFFF#11.22", "1#80:0:0#"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		x, err := canbus.ParseXLFrame(s)
		if err != nil {
			return
		}
		text := toXLJSON(&x).text()
		y, err := canbus.ParseXLFrame(text)
		if err != nil {
			t.Fatalf("%q printed as %q, which does not parse: %v", s, text, err)
		}
		if x.VCID != y.VCID || x.Priority != y.Priority || x.SDT != y.SDT || x.AF != y.AF || x.Flags != y.Flags || !slices.Equal(x.Data, y.Data) {
			t.Fatalf("%q → %q: %+v, read back %+v", s, text, x, y)
		}
	})
}

// FuzzSignalValue: any physical value a decoder yields is printed in
// text and in valid JSON (NaN and infinities as strings).
func FuzzSignalValue(f *testing.F) {
	for _, v := range []float64{0, -40, 0.1 * 3, 1e300, math.Inf(1), math.Inf(-1), math.NaN(), math.SmallestNonzeroFloat64} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v float64) {
		s := &candb.Signal{Name: "S", Length: 8, Factor: 1, Unit: "u"}
		sigs := decodeSignals([]candb.DecodedSignal{{Signal: s, Value: candb.Value{Raw: 1, Signed: 1, Physical: v}}})
		b, err := json.Marshal(sigs)
		if err != nil || !json.Valid(b) {
			t.Fatalf("%v: %s %v", v, b, err)
		}
		h := human(v)
		if _, err := strconv.ParseFloat(h, 64); err != nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			t.Fatalf("%v printed as %q", v, h)
		}
		if !strings.HasPrefix(signalsText(sigs), "S="+h+" u") || signed(v) == "" {
			t.Fatal(signalsText(sigs))
		}
	})
}

// FuzzGoPackageName: the package name derived from a database file name
// is a Go identifier that is not a keyword.
func FuzzGoPackageName(f *testing.F) {
	for _, s := range []string{"vehicle.dbc", "My-Car_2.kcd", "123.dbc", "func.dbc", "/x/.dbc", "ÄÖ.arxml"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, path string) {
		p := goPackageName(path)
		if !token.IsIdentifier(p) || token.IsKeyword(p) {
			t.Fatalf("%q: package %q", path, p)
		}
	})
}

// FuzzSlcanFlags: --slcan-bitrate and --slcan-data-bitrate take the
// adapter's rates or 0; anything else is a usage-level refusal (exit 3)
// before a command runs.
func FuzzSlcanFlags(f *testing.F) {
	f.Add("500000", "0", "115200")
	f.Add("-1", "2000000", "0")
	f.Add("x", "", "9999999999999999999")
	f.Fuzz(func(t *testing.T, br, dbr, baud string) {
		args := []string{"version", "--format", "json", "--slcan-bitrate", br, "--slcan-data-bitrate", dbr, "--slcan-baud", baud}
		r := runQuick(nil, time.Second, args...)
		checkJSONOutput(t, args, r)
		if r.code != 0 && r.code != 2 && r.code != 3 {
			t.Fatalf("%q: exit %d", args, r.code)
		}
	})
}

// FuzzSendLimits: send on the virtual bus with fuzzed --rate,
// --max-frames, --duration, --repeat and --gap. Whatever is accepted, no
// more than --max-frames frames go out, no faster than --rate, and the
// exit code is documented.
func FuzzSendLimits(f *testing.F) {
	f.Add("0", "10000", "0s", "1", "0s", uint8(1))
	f.Add("-1", "-5", "-1s", "4611686018427387904", "0", uint8(2))
	f.Add("NaN", "3", "10ms", "2", "1ms", uint8(3))
	f.Add("+Inf", "0", "1ns", "9223372036854775807", "-1ms", uint8(1))
	f.Add("1e-300", "2", "1h", "1", "1h", uint8(1))
	f.Fuzz(func(t *testing.T, rate, maxFrames, duration, repeat, gap string, n uint8) {
		args := []string{"send", "-i", "virtual", "--format", "json", "--rate", rate, "--max-frames", maxFrames,
			"--duration", duration, "--repeat", repeat, "--gap", gap}
		for i := range int(n%4) + 1 {
			args = append(args, "1"+strconv.Itoa(i)+"#00")
		}
		r := runQuick(nil, 100*time.Millisecond, args...)
		checkJSONOutput(t, args, r)
		if r.code != 0 {
			return
		}
		sent := int(jsonDoc(t, r.out)["result"].(map[string]any)["sent"].(float64))
		if m, err := strconv.Atoi(maxFrames); err == nil && m > 0 && sent > m {
			t.Fatalf("%q: %d frames sent, --max-frames %d", args, sent, m)
		}
		if mf, _ := strconv.Atoi(maxFrames); mf < 0 {
			t.Fatalf("%q: negative --max-frames accepted", args)
		}
		// The pacer spaces frames 1/rate apart: within the 100 ms deadline
		// (generously 200 ms) no more than the first plus rate × 0.2 go out.
		if rt, err := strconv.ParseFloat(rate, 64); err == nil && rt > 0 && float64(sent) > 2+rt*0.2 {
			t.Fatalf("%q: %d frames sent at --rate %v", args, sent, rt)
		}
	})
}

// readOnlyCommands run on a fuzzed file; {} is the file, {out} a fresh
// output path in a temporary directory.
var readOnlyCommands = [][]string{
	{"trace", "info", "{}"},
	{"trace", "convert", "{}", "{out}.pcap"},
	{"trace", "convert", "{}", "{out}.pcapng"},
	{"trace", "convert", "{}", "{out}.blf"},
	{"trace", "convert", "{}", "{out}.asc"},
	{"trace", "convert", "{}", "{out}.trc"},
	{"trace", "convert", "{}", "{out}.log"},
	{"trace", "convert", "{}", "{out}.jsonl"},
	{"dbc", "inspect", "{}"},
	{"dbc", "validate", "{}"},
	{"dbc", "decode", "{}", "123#1122334455667788", "18FEF1FE#00E8030000000000", "200##1" + strings.Repeat("AB", 12)},
	{"dbc", "encode", "{}", "100"},
	{"dbc", "convert", "{}", "--to", "dbc", "-o", "{out}.dbc"},
	{"dbc", "convert", "{}", "--to", "json"},
	{"dbc", "generate", "{}", "-o", "{out}.go"},
}

// fuzzFileExt is the extension of the fuzzed file: it selects the
// database parser (content sniffing decides for the others and traces).
var fuzzFileExt = []string{".log", ".dbc", ".kcd", ".arxml", ".xml", ""}

// FuzzReadOnlyCommands runs every command that only reads its input on
// fuzzed file contents, through cmd.Run with --format json: no panic, a
// documented exit code, valid JSON, no "internal_unexpected", and a
// failed trace convert leaves no output file behind.
func FuzzReadOnlyCommands(f *testing.F) {
	for _, p := range []string{"testdata/trace.log", "testdata/vehicle.dbc", "testdata/sample.kcd", "testdata/system.arxml", "testdata/bad.dbc", "testdata/floats.dbc"} {
		if b, err := os.ReadFile(p); err == nil {
			for c := range readOnlyCommands {
				f.Add(uint8(c), uint8(slices.Index(fuzzFileExt, filepath.Ext(p))), b)
			}
		}
	}
	f.Add(uint8(0), uint8(0), []byte("(99999999999.000000) can0 123#00\n"))
	f.Add(uint8(1), uint8(0), []byte("(1700000000.000000) 300 123##1"+strings.Repeat("00", 64)+"\n"))
	f.Fuzz(func(t *testing.T, cmdIdx, extIdx uint8, data []byte) {
		dir := t.TempDir()
		in := filepath.Join(dir, "in"+fuzzFileExt[int(extIdx)%len(fuzzFileExt)])
		if err := os.WriteFile(in, data, 0o600); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(dir, "out")
		var args []string
		for _, a := range readOnlyCommands[int(cmdIdx)%len(readOnlyCommands)] {
			a = strings.ReplaceAll(a, "{out}", out)
			args = append(args, strings.ReplaceAll(a, "{}", in))
		}
		args = append(args, "--format", "json")
		r := runQuick(nil, 5*time.Second, args...)
		checkJSONOutput(t, args, r)
		if r.code == 130 { // only the 5 s deadline cancels a command that reads a file
			t.Fatalf("%q on %d bytes ran into the deadline", args, len(data))
		}
		if args[0] == "trace" && args[1] == "convert" && r.code != 0 {
			if _, err := os.Stat(args[3]); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%q: exit %d left %s", args, r.code, args[3])
			}
		}
	})
}

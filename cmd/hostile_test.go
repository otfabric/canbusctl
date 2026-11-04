// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// noFile fails when path exists.
func noFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s exists after a failed command (%v)", path, err)
	}
}

// writeFile writes a test input.
func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// ascFrom converts testdata/trace.log (2023) to ASC and moves its dates to
// year.
func ascFrom(t *testing.T, year string) string {
	t.Helper()
	asc := filepath.Join(t.TempDir(), "t.asc")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", asc), 0)
	b, err := os.ReadFile(asc)
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, "old.asc", strings.ReplaceAll(string(b), " 2023", " "+year))
}

// TestTraceConvertUnrepresentable: go-canbus v0.8.0 refuses to write a
// frame its format cannot hold (it wrapped before). trace convert reports
// it as invalid input (exit 3, unrepresentable_frame, the record and the
// reason) and leaves no truncated file behind, also where an older output
// existed.
func TestTraceConvertUnrepresentable(t *testing.T) {
	old := ascFrom(t, "1960")
	dir := t.TempDir()
	for ext, reason := range map[string]string{".pcap": "outside 1970..2106", ".pcapng": "outside 1970..2554", ".log": "outside 1970..9999", ".jsonl": "outside 1970..9999"} {
		out := filepath.Join(dir, "o"+ext)
		r := run(t, "", "trace", "convert", old, out, "--format", "json")
		want(t, r, 3, `"code": "unrepresentable_frame"`, "record 1", reason)
		noFile(t, out)
	}
	// BLF holds 1601..30827: the 1960 trace converts.
	want(t, run(t, "", "trace", "convert", old, filepath.Join(dir, "o.blf")), 0, "4 written")
	// A previous output of the same name is not left half-overwritten.
	prev := filepath.Join(dir, "prev.pcap")
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", prev), 0)
	want(t, run(t, "", "trace", "convert", old, prev), 3, "1970..2106")
	noFile(t, prev)

	// A CAN FD frame on channel 300: BLF's CAN_FD_MESSAGE_64 has an 8-bit
	// channel.
	fd := writeFile(t, "fd.log", "(1700000000.000000) 300 123##1"+strings.Repeat("00", 64)+"\n")
	r := run(t, "", "trace", "convert", fd, filepath.Join(dir, "fd.blf"))
	want(t, r, 3, "BLF CAN FD channel 300")
	noFile(t, filepath.Join(dir, "fd.blf"))
	// Classical frames have 16 bits; a channel beyond them becomes 1, which
	// is reported as a loss.
	ch := writeFile(t, "ch.log", "(1700000000.000000) 70000 123#11\n(1700000000.000000) 0 124#11\n(1700000000.000000) 300 125#11\n")
	want(t, run(t, "", "trace", "convert", ch, filepath.Join(dir, "ch.blf")), 0, "3 written", "lost: 2 frames: interface name")
	want(t, run(t, "", "trace", "convert", ch, filepath.Join(dir, "ch.asc")), 0, "3 written")
}

// TestTraceOutOfRangeTimestamps: readers of go-canbus v0.8.0 reject
// timestamps beyond the year 9999 with a ParseError: exit 3
// (invalid_capture), with the line, and no output.
func TestTraceOutOfRangeTimestamps(t *testing.T) {
	far := writeFile(t, "far.log", "(1700000000.000000) can0 123#00\n(99999999999999.000000) can0 123#00\n")
	want(t, run(t, "", "trace", "info", far, "--format", "json"), 3, `"code": "invalid_capture"`, "record 2", "after the year 9999")
	out := filepath.Join(t.TempDir(), "o.pcap")
	want(t, run(t, "", "trace", "convert", far, out), 3, "after the year 9999")
	noFile(t, out)
	neg := writeFile(t, "neg.log", "(-5.000000) can0 123#00\n")
	want(t, run(t, "", "trace", "info", neg), 3, "bad timestamp")
	want(t, run(t, "", "dbc", "decode", "testdata/vehicle.dbc", "--trace", far), 3, "after the year 9999")
	want(t, run(t, "", "inject", "--trace", far), 3, "after the year 9999")
	// Documented, not changed: go-canbus ignores an ASC date header it
	// cannot parse (year 10000) and reads the times relative to 1970.
	want(t, run(t, "", "trace", "info", ascFrom(t, "10000"), "--format", "json"), 0, `"first": "19`)
}

// TestTraceConvertSameFile: converting a file onto itself (or onto a link
// to it) truncated the input it was reading; it is refused and the input
// kept.
func TestTraceConvertSameFile(t *testing.T) {
	in := writeFile(t, "in.log", "(1700000000.000000) can0 123#00\n")
	want(t, run(t, "", "trace", "convert", in, in), 3, "same file")
	alias := filepath.Join(filepath.Dir(in), "sub", "..", "in.log")
	want(t, run(t, "", "trace", "convert", in, alias, "--file-format", "pcap"), 3, "same file")
	if err := os.Symlink(in, filepath.Join(filepath.Dir(in), "link.log")); err == nil {
		want(t, run(t, "", "trace", "convert", in, filepath.Join(filepath.Dir(in), "link.log")), 3, "same file")
	}
	if b, _ := os.ReadFile(in); string(b) != "(1700000000.000000) can0 123#00\n" {
		t.Fatalf("input changed: %q", b)
	}
}

// TestOutputPaths: ".." is resolved as given; an output path that is a
// symbolic link is refused (exit 3, file_open_failed) by every command
// that writes a file, and neither the link nor its target is touched.
func TestOutputPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", filepath.Join(dir, "a", "..", "up.log")), 0)
	if _, err := os.Stat(filepath.Join(dir, "up.log")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.log")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symbolic links:", err)
	}
	dangling := filepath.Join(dir, "dangling.log")
	if err := os.Symlink(filepath.Join(dir, "nowhere.log"), dangling); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{link, dangling} {
		for _, args := range [][]string{
			{"trace", "convert", "testdata/trace.log", out},
			{"trace", "convert", ascFrom(t, "1960"), out},
			{"dbc", "generate", "testdata/vehicle.dbc", "-o", out},
			{"dbc", "convert", "testdata/vehicle.dbc", "--to", "dbc", "-o", out},
			{"record", "-i", "virtual", "--duration", "10ms", "-o", out},
		} {
			want(t, run(t, "", append(args, "--format", "json")...), 3, `"code": "file_open_failed"`, "symbolic link")
		}
	}
	st, _ := os.Lstat(link)
	if b, _ := os.ReadFile(target); st.Mode()&os.ModeSymlink == 0 || string(b) != "old" {
		t.Fatalf("link %v, target %q", st.Mode(), b)
	}
	if _, err := os.Lstat(filepath.Join(dir, "nowhere.log")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a dangling link's target was created")
	}
	// The target itself, named directly, is written and keeps its mode.
	want(t, run(t, "", "trace", "convert", "testdata/trace.log", target), 0, "4 written")
	if tst, _ := os.Stat(target); tst.Size() < 50 || runtime.GOOS != "windows" && tst.Mode().Perm() != 0o644 {
		t.Fatalf("target %v %o", tst.Size(), tst.Mode().Perm())
	}
}

// TestHostileDatabases: huge, deep and out-of-range databases end with a
// documented code, quickly, without an internal error.
func TestHostileDatabases(t *testing.T) {
	var b strings.Builder
	b.WriteString("VERSION \"\"\nBS_:\nBU_: ECU\n")
	for i := range 3000 { // 2048 standard identifiers, then extended ones
		id := uint32(i)
		if i > 0x7FF {
			id |= 0x80000000
		}
		fmt.Fprintf(&b, "BO_ %d M%d: 8 ECU\n", id, i)
		for s := range 8 {
			fmt.Fprintf(&b, " SG_ S%d_%d : %d|8@1+ (1,0) [0|255] \"\" ECU\n", i, s, s*8)
		}
	}
	big := writeFile(t, "big.dbc", b.String())
	began := time.Now()
	want(t, run(t, "", "dbc", "inspect", big, "--format", "json"), 0, `"M2999"`)
	want(t, run(t, "", "dbc", "validate", big), 0, "valid")
	if el := time.Since(began); el > 20*time.Second {
		t.Fatalf("3000 messages took %v", el)
	}
	// One 4 MiB comment line.
	want(t, run(t, "", "dbc", "inspect", writeFile(t, "long.dbc", "VERSION \"\"\nBS_:\nBU_: ECU\nBO_ 1 M: 8 ECU\nCM_ BO_ 1 \""+strings.Repeat("x", 4<<20)+"\";\n")), 0)
	// Message identifiers beyond 32 bits, in any statement (v0.8.0).
	for _, stmt := range []string{"BO_ 4294967296 X: 8 ECU", "CM_ BO_ 4294967296 \"c\";", "BA_ \"GenMsgCycleTime\" BO_ 99999999999 10;", "VAL_ 4294967297 S 0 \"a\";", "SIG_VALTYPE_ 4294967296 S : 1;"} {
		p := writeFile(t, "id.dbc", "VERSION \"\"\nBS_:\nBU_: ECU\nBO_ 1 M: 8 ECU\n SG_ S : 0|8@1+ (1,0) [0|0] \"\" ECU\n"+stmt+"\n")
		r := run(t, "", "dbc", "inspect", p, "--format", "json")
		if r.code != 0 && r.code != 3 || strings.Contains(r.out, "internal") {
			t.Fatalf("%s: exit %d %s", stmt, r.code, r.out)
		}
		if strings.HasPrefix(stmt, "BO_") {
			want(t, r, 3, "invalid_database")
		}
	}
	// Deep XML: KCD and ARXML nested 50000 levels.
	deep := strings.Repeat("<a>", 50000) + strings.Repeat("</a>", 50000)
	for name, src := range map[string]string{
		"deep.kcd":   "<NetworkDefinition>" + deep + "</NetworkDefinition>",
		"deep.arxml": "<AUTOSAR>" + deep + "</AUTOSAR>",
		"open.kcd":   "<NetworkDefinition>" + strings.Repeat("<Bus name=\"b\">", 50000),
	} {
		r := run(t, "", "dbc", "inspect", writeFile(t, name, src), "--format", "json")
		if r.code != 0 && r.code != 3 || strings.Contains(r.out, "internal") {
			t.Fatalf("%s: exit %d %s", name, r.code, r.out)
		}
	}
	// Many signals in one CAN FD message.
	var many strings.Builder
	many.WriteString("VERSION \"\"\nBS_:\nBU_: ECU\nBO_ 1 M: 64 ECU\n")
	for s := range 512 {
		fmt.Fprintf(&many, " SG_ S%d : %d|1@1+ (1,0) [0|1] \"\" ECU\n", s, s)
	}
	p := writeFile(t, "many.dbc", many.String())
	want(t, run(t, "", "dbc", "decode", p, "001##0"+strings.Repeat("FF", 64), "--format", "json"), 0, `"S511"`)
}

// TestHostileStdin: binary garbage and over-long lines on stdin are
// invalid input (exit 3), never an internal error, and nothing is
// transmitted or written.
func TestHostileStdin(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	garbage := make([]byte, 1<<16)
	rng.Read(garbage)
	long := strings.Repeat("1", 70000) + "#00\n"
	out := filepath.Join(t.TempDir(), "o.pcap")
	for _, in := range []string{string(garbage), long, "\x00\x00\x00\x00", "\xa1\xb2\xc3\xd4" + string(garbage[:100])} {
		for _, args := range [][]string{
			{"trace", "info", "-"},
			{"trace", "convert", "-", out},
			{"dbc", "decode", "testdata/vehicle.dbc"},
			{"dbc", "decode", "testdata/vehicle.dbc", "--trace", "-"},
			{"replay", "-i", "virtual", "--max-speed", "-"},
			{"inject", "--trace", "-"},
		} {
			r := run(t, in, append(args, "--format", "json")...)
			if r.code != 3 && (args[0] != "replay" || r.code != 10) {
				t.Fatalf("%q on %d bytes: exit %d\n%s%s", args, len(in), r.code, r.out, r.errOut)
			}
			if args[0] != "replay" && strings.Contains(r.out, "internal_unexpected") {
				t.Fatalf("%q: %s", args, r.out)
			}
			noFile(t, out)
		}
	}
}

// TestReplayMalformedTrace: replay reads a whole file before its first
// frame, so a malformed record anywhere exits 3 with nothing sent. From
// standard input it cannot: a bad record after frames were sent exits 10
// (invalid_capture, with the count sent), one before any exits 3.
func TestReplayMalformedTrace(t *testing.T) {
	bad := "(1700000000.000000) can0 100#11\n(1700000000.000001) can0 garbage\n"
	file := writeFile(t, "bad.log", bad)
	r := run(t, "", "replay", "-i", "virtual", "--max-speed", file, "--format", "json")
	want(t, r, 3, `"code": "invalid_capture"`, "record 2", "nothing sent")
	r = run(t, bad, "replay", "-i", "virtual", "--max-speed", "-", "--format", "json")
	want(t, r, 10, `"code": "invalid_capture"`, "record 2", "after 1 frames sent")
	r = run(t, "garbage\n", "replay", "-i", "virtual", "--max-speed", "-", "--format", "json")
	want(t, r, 3, `"code": "invalid_capture"`)
}

// TestTransmitBounds: the bounds of transmitting commands are enforced
// before anything is sent: negative or NaN values were taken as "no
// limit"; --repeat × frames overflowed past --max-frames; --malformed
// and isotp send --size allocated what they were given.
func TestTransmitBounds(t *testing.T) {
	cmds := [][]string{
		{"send", "-i", "virtual", "123#00"},
		{"send-fd", "-i", "virtual", "123##0"},
		{"xl", "send", "-i", "virtual", "123#80:00:00000000#00"},
		{"replay", "-i", "virtual", "--max-speed", "testdata/trace.log"},
		{"simulate", "--dbc", "testdata/vehicle.dbc", "--duration", "50ms"},
		{"inject", "--frame", "123#00"},
	}
	for _, c := range cmds {
		for _, bad := range [][]string{{"--rate", "-1"}, {"--rate", "NaN"}, {"--rate", "+Inf"}, {"--max-frames", "-1"}, {"--duration", "-1s"}} {
			r := run(t, "", append(append([]string{}, c...), bad...)...)
			want(t, r, 3, strings.TrimPrefix(bad[0], "--"))
			if strings.Contains(r.out, "sent") && !strings.Contains(r.out, "sent 0") {
				t.Fatalf("%q %q sent: %s", c, bad, r.out)
			}
		}
		// The bounds still accept 0 (no limit) and positive values.
		want(t, run(t, "", append(append([]string{}, c...), "--rate", "0", "--max-frames", "0")...), 0)
	}
	// 2^62 × 2 frames is 2^63: it wrapped negative and passed --max-frames.
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "4611686018427387904", "123#00", "123#00"), 3, "exceed --max-frames 10000")
	want(t, run(t, "", "xl", "send", "-i", "virtual", "--repeat", "4611686018427387904", "123#80:00:00000000#00", "123#80:00:00000000#00"), 3, "exceed --max-frames")
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "5000", "123#00", "123#00"), 0, "sent 10000 frame(s)")
	want(t, run(t, "", "send", "-i", "virtual", "--repeat", "5001", "123#00", "123#00"), 3, "exceed --max-frames 10000")
	// A tiny --rate is a long interval, not "no limit" (the float to
	// Duration conversion overflowed): the second frame waits.
	r := run(t, "", "send", "-i", "virtual", "--rate", "1e-300", "--duration", "200ms", "123#00", "123#01")
	want(t, r, 0, "sent 1 frame(s)")

	want(t, run(t, "", "inject", "--malformed", "-1", "--frame", "123#00"), 3, "--malformed -1")
	want(t, run(t, "", "inject", "--malformed", "2000000000"), 3, "--malformed 2000000000")
	want(t, run(t, "", "inject", "--malformed", "20000"), 3, "exceed --max-frames 10000")
	want(t, run(t, "", "inject", "--malformed", "20000", "--max-frames", "0", "--format", "json"), 0, `"malformed": 20000`)
	want(t, run(t, "", "inject", "--frame", "123#00", "--frame", "124#00", "--max-frames", "1"), 3, "exceed --max-frames 1")
	// NaN probabilities compared false and were taken as 0.
	want(t, run(t, "", "inject", "--frame", "123#00", "--drop", "NaN"), 3, "between 0 and 1")
	want(t, run(t, "", "bridge", "-a", "virtual", "-b", "vcan0", "--duration", "1s", "--corrupt", "NaN"), 3, "between 0 and 1")

	// isotp send --size beyond --max-size is refused before allocating
	// (it exhausted memory first); the exit code is that of Send's own
	// refusal (6, ErrTooLarge).
	began := time.Now()
	want(t, run(t, "", "isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "900000000000"), 6, "at most 4095")
	want(t, run(t, "", "isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "900000000000", "--max-size", "99999999999"), 6, "at most 4294967295")
	want(t, run(t, "", "isotp", "send", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "5000", "--max-size", "1000"), 6, "at most 1000")
	if el := time.Since(began); el > 5*time.Second {
		t.Fatalf("refusals took %v", el)
	}
	if sendSizeLimit(0) != 4095 || sendSizeLimit(-1) != 4095 || sendSizeLimit(64) != 64 {
		t.Fatal("sendSizeLimit")
	}
}

// TestInjectMalformedTrace: a record that does not parse ended the
// trace silently and inject went on with part of it (exit 0).
func TestInjectMalformedTrace(t *testing.T) {
	bad := writeFile(t, "bad.log", "(1700000000.000000) can0 100#11\n(1700000000.000001) can0 garbage\n")
	want(t, run(t, "", "inject", "--trace", bad, "--format", "json"), 3, `"code": "invalid_capture"`, "record 2")
	// CAN XL records are skipped, not an error.
	xl := writeFile(t, "xl.log", "(1700000000.000000) can0 100#11\n(1700000000.000001) can0 00242#80:03:00C0FFEE#11223344\n(1700000000.000002) can0 101#22\n")
	want(t, run(t, "", "inject", "--trace", xl), 0, "2 source frames, 2 delivered")
}

// TestGeneratePackageName: func.dbc gave "package func", which codegen
// reported as a generator bug with the whole source in the message.
func TestGeneratePackageName(t *testing.T) {
	src, err := os.ReadFile("testdata/vehicle.dbc")
	if err != nil {
		t.Fatal(err)
	}
	p := writeFile(t, "func.dbc", string(src))
	want(t, run(t, "", "dbc", "generate", p), 0, "package funcdb")
	r := run(t, "", "dbc", "generate", p, "--package", "type")
	want(t, r, 3, `invalid package name "type"`)
	if len(r.errOut) > 200 {
		t.Fatalf("error of %d bytes", len(r.errOut))
	}
	want(t, run(t, "", "dbc", "generate", p, "--package", "a-b"), 3, "invalid package")
	for in, w := range map[string]string{"func.dbc": "funcdb", "Type.kcd": "typedb", "x.dbc": "x", "9.dbc": "candb"} {
		if got := goPackageName(in); got != w {
			t.Errorf("goPackageName(%q) = %q, want %q", in, got, w)
		}
	}
}

// TestDecodeLongStdinLine: a stdin line beyond 64 KiB was an internal
// error (exit 10).
func TestDecodeLongStdinLine(t *testing.T) {
	want(t, run(t, strings.Repeat("A", 70000)+"\n", "dbc", "decode", "testdata/vehicle.dbc", "--format", "json"), 3, `"code": "invalid_input"`, "token too long")
}

// TestTraceConvertWriteErrors: a CAN XL frame the target cannot hold
// exits 3 (unrepresentable_frame); a device that refuses the data (Linux
// /dev/full) is an output failure, exit 10.
func TestTraceConvertWriteErrors(t *testing.T) {
	xl := writeFile(t, "xl.log", "(5000000000.000000) vcan0 123#80:00:00000000#00\n")
	out := filepath.Join(t.TempDir(), "xl.pcap")
	want(t, run(t, "", "trace", "convert", xl, out, "--format", "json"), 3, `"code": "unrepresentable_frame"`, "1970..2106")
	noFile(t, out)
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full")
	}
	for _, ff := range []string{"candump", "pcap", "blf"} {
		want(t, run(t, "", "trace", "convert", "testdata/trace.log", "/dev/full", "--file-format", ff, "--format", "json"), 10, `"code": "output_failed"`)
	}
	want(t, run(t, "", "dbc", "generate", "testdata/vehicle.dbc", "-o", "/dev/full", "--format", "json"), 10, `"code": "output_failed"`)
}

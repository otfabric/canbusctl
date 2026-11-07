// SPDX-License-Identifier: MIT

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
)

func FuzzParseID(f *testing.F) {
	for _, s := range []string{"7FF", "0x18DAF110", "0X1", " 123 ", "800", "-1", "+5", "0x", "1FFFFFFF", "20000000"} {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, s string, extended bool) {
		id, err := ParseID(s, extended)
		if err != nil {
			if e, ok := errors.AsType[*errs.Error](err); !ok || e.Kind != errs.KindInvalidInput {
				t.Fatalf("%q: untyped error %v", s, err)
			}
			return
		}
		max := canbus.MaxStandardID
		if extended {
			max = canbus.MaxExtendedID
		}
		if id > max {
			t.Fatalf("%q: %X accepted beyond %X", s, uint32(id), uint32(max))
		}
	})
}

func FuzzParseHexByte(f *testing.F) {
	for _, s := range []string{"CC", "0xff", "0X00", "100", "", "-1", "+1", "g"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, err := ParseHexByte(s)
		if err != nil {
			return
		}
		// What is accepted is a hexadecimal byte, at most with a 0x prefix
		// (and the sign strconv takes).
		t2 := strings.TrimLeft(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), "+")
		if t2 == "" || len(strings.TrimLeft(t2, "0")) > 2 {
			t.Fatalf("%q accepted as %02X", s, b)
		}
	})
}

// FuzzFrameLine: a frame canbusctl accepts prints as a candump line that
// go-canbus reads back as the same frame.
func FuzzFrameLine(f *testing.F) {
	for _, s := range []string{"123#DEADBEEF", "12345678##3AABB", "123#R5", "123#1122334455667788_C", "7FF#"} {
		f.Add(s, int64(1700000000), int64(123456000))
	}
	f.Fuzz(func(t *testing.T, s string, sec, nsec int64) {
		fr, err := ParseFrame(s)
		if err != nil {
			return
		}
		sec = max(0, min(sec, 253402300799))
		nsec = max(0, min(nsec, 999999999)) / 1000 * 1000
		fr.Timestamp = time.Unix(sec, nsec)
		fr.Interface = "can0"
		line := FrameLine(&fr)
		var back canbus.Frame
		if err := capture.ParseCandumpLine(line, &back); err != nil {
			t.Fatalf("%q printed as %q, which does not parse: %v", s, line, err)
		}
		if back.String() != fr.String() || !back.Timestamp.Equal(fr.Timestamp) || back.Interface != "can0" {
			t.Fatalf("%q: %q read back as %v", s, line, back)
		}
		if j := ToJSON(&fr); j.Len != int(fr.Len) || len(j.Data) != 2*len(fr.Payload()) {
			t.Fatalf("%q: JSON %+v", s, j)
		}
	})
}

// FuzzReadLines: every line returned is trimmed, not empty, and not a
// comment; very long lines are an error, not a silent truncation.
func FuzzReadLines(f *testing.F) {
	f.Add("123#00\n\n# c\n 456#11 \r\n")
	f.Add(strings.Repeat("A", 70000))
	f.Fuzz(func(t *testing.T, s string) {
		lines, err := ReadLines(strings.NewReader(s))
		if err != nil {
			return
		}
		for _, l := range lines {
			if l == "" || l != strings.TrimSpace(l) || strings.HasPrefix(l, "#") || strings.Contains(l, "\n") {
				t.Fatalf("line %q", l)
			}
		}
	})
}

// FuzzTransmitCheckSlcan: a serial-line adapter never transmits without
// --yes, whatever follows the prefix.
func FuzzTransmitCheckSlcan(f *testing.F) {
	for _, s := range []string{"/dev/ttyACM0", "COM3", "tcp://127.0.0.1:1", "", "../../x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		err := TransmitCheck(SlcanPrefix+s, false, "send")
		if e, ok := errors.AsType[*errs.Error](err); !ok || e.Kind != errs.KindSafety || errs.ExitCode(e.Kind) != 13 {
			t.Fatalf("%q: %v", s, err)
		}
		if TransmitCheck(SlcanPrefix+s, true, "send") != nil {
			t.Fatal("--yes refused")
		}
	})
}

func TestReadLinesTooLong(t *testing.T) {
	// bufio.Scanner's 64 KiB limit: an error (exit 10 through the
	// commands), never a truncated frame.
	if _, err := ReadLines(strings.NewReader(strings.Repeat("1", 70000) + "\n")); err == nil {
		t.Fatal("a 70000-byte line was read")
	}
}

func TestSameFileAndRemovePartial(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.log")
	if err := os.WriteFile(a, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !SameFile(a, filepath.Join(dir, "sub", "..", "a.log")) || SameFile(a, filepath.Join(dir, "b.log")) || SameFile(filepath.Join(dir, "none"), a) {
		t.Fatal("SameFile")
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "l.log")
		if err := os.Symlink(a, link); err != nil {
			t.Fatal(err)
		}
		if !SameFile(a, link) {
			t.Fatal("a link to the input is the input")
		}
		RemovePartial(link) // a link is left alone, and so is its target
		if _, err := os.Lstat(link); err != nil {
			t.Fatal("link removed")
		}
		if _, err := os.Stat(a); err != nil {
			t.Fatal("target removed")
		}
		RemovePartial(dir) // not a regular file
		if _, err := os.Stat(dir); err != nil {
			t.Fatal("directory removed")
		}
	}
	RemovePartial(a)
	if _, err := os.Stat(a); !os.IsNotExist(err) {
		t.Fatal("partial file kept")
	}
	RemovePartial(a) // gone: nothing to do
}

func TestCreateOutputMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX permissions")
	}
	p := filepath.Join(t.TempDir(), "o.log")
	f, err := CreateOutput(p)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if st, _ := os.Stat(p); st.Mode().Perm() != OutputMode {
		t.Fatalf("new file: mode %o", st.Mode().Perm())
	}
	// Documented, not changed: an existing file is truncated and keeps
	// its mode (O_TRUNC does not chmod), as with shell redirection.
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	if f, err = CreateOutput(p); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o644 {
		t.Fatalf("existing file: mode %o", st.Mode().Perm())
	}
}

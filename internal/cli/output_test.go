// SPDX-License-Identifier: MIT

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/capture"
)

func TestCreateOutputRefusesLinks(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "t")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "l")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("no symbolic links:", err)
	}
	if f, err := CreateOutput(link); !errors.Is(err, ErrOutputLink) {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("link: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "keep" {
		t.Fatalf("target changed: %q", b)
	}
	if !OutputIsLink(link) || OutputIsLink(target) || OutputIsLink(filepath.Join(dir, "none")) {
		t.Error("OutputIsLink")
	}
	f, err := CreateOutput(filepath.Join(dir, "new"))
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestCheckTrace(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "g.log")
	bad := filepath.Join(dir, "b.log")
	_ = os.WriteFile(good, []byte("(1.000000) can0 123#00\n"), 0o600)
	_ = os.WriteFile(bad, []byte("(1.000000) can0 123#00\n(2.000000) can0 zz\n"), 0o600)
	if err := CheckTrace(good); err != nil {
		t.Errorf("good: %v", err)
	}
	if err := CheckTrace("-"); err != nil {
		t.Errorf("stdin: %v", err)
	}
	err := CheckTrace(bad)
	if e := errs.Classify(err); e.Code != errs.CodeInvalidCapture || e.Kind != errs.KindInvalidInput {
		t.Errorf("bad: %v (%s/%s)", err, e.Kind, e.Code)
	}
	if e := errs.Classify(CheckTrace(filepath.Join(dir, "missing"))); e.Code != errs.CodeFileOpen {
		t.Errorf("missing: %s", e.Code)
	}
}

// TestCheckTraceNonXLReader: formats without CAN XL (ASC) are checked
// through Read.
func TestCheckTraceNonXLReader(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.asc")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	w, err := capture.NewWriter(f, capture.FormatASC)
	if err != nil {
		t.Fatal(err)
	}
	fr := canbus.Frame{ID: 0x123, Len: 1, Timestamp: time.Unix(1700000000, 0)}
	if err := w.Write(&fr); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	_ = f.Close()
	if err := CheckTrace(p); err != nil {
		t.Fatal(err)
	}
}

func TestWriteOutput(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "o")
	if err := WriteOutput(p, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "x" {
		t.Fatalf("%q", b)
	}
	if err := WriteOutput(filepath.Join(dir, "no", "o"), nil); err == nil {
		t.Error("missing directory accepted")
	}
	if _, err := os.Stat("/dev/full"); err == nil {
		if err := WriteOutput("/dev/full", []byte("x")); err == nil {
			t.Error("write to /dev/full succeeded")
		}
	}
}

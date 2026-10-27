// SPDX-License-Identifier: MIT

// Package cli holds what several canbusctl commands share: parsing of
// identifiers, frames and signal assignments, loading databases and
// traces, opening interfaces and the transmit safety check.
package cli

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/capture"
	"github.com/otfabric/go-canbus/dbc"
	"github.com/otfabric/go-canbus/kcd"
	"github.com/otfabric/go-canbus/socketcan/link"
)

// Virtual is the interface name of canbusctl's in-process virtual bus.
const Virtual = "virtual"

// ParseID parses a hexadecimal identifier ("123", "0x18DAF110").
func ParseID(s string, extended bool) (canbus.ID, error) {
	t := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "0x"), "0X")
	v, err := strconv.ParseUint(t, 16, 32)
	max := canbus.MaxStandardID
	if extended {
		max = canbus.MaxExtendedID
	}
	if err != nil || canbus.ID(v) > max {
		return 0, errs.Invalid("invalid %s identifier %q (hexadecimal, at most %X)", map[bool]string{true: "29-bit", false: "11-bit"}[extended], s, uint32(max))
	}
	return canbus.ID(v), nil
}

// ParseHexByte parses a byte given in hexadecimal.
func ParseHexByte(s string) (byte, error) {
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"), 16, 8)
	if err != nil {
		return 0, errs.Invalid("invalid byte %q (hexadecimal 00..FF)", s)
	}
	return byte(v), nil
}

// ParseFrame parses cansend notation into a validated frame.
func ParseFrame(s string) (canbus.Frame, error) {
	var f canbus.Frame
	if err := capture.ParseFrame(strings.TrimSpace(s), &f); err != nil {
		return f, &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidFrame, Message: err.Error(), Cause: err}
	}
	if f.IsError() {
		return f, errs.New(errs.KindInvalidInput, errs.CodeInvalidFrame, "error frames cannot be sent")
	}
	if err := f.Validate(); err != nil {
		return f, &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidFrame, Message: err.Error(), Cause: err}
	}
	return f, nil
}

// ParseFilters parses --filter values.
func ParseFilters(specs []string, join bool, errorMask bool) (canbus.FilterSet, error) {
	fs := canbus.FilterSet{Join: join}
	for _, s := range specs {
		f, err := canbus.ParseFilter(s)
		if err != nil {
			return fs, errs.Invalid("invalid --filter: %v", err)
		}
		fs.Filters = append(fs.Filters, f)
	}
	if errorMask {
		fs.ErrorMask = canbus.ErrClassAll
	}
	return fs, nil
}

// ParseAssignments parses "signal=value" pairs. A value is a number
// (physical, or raw with raw), or a value description.
func ParseAssignments(pairs []string, raw bool) (map[string]candb.Input, error) {
	out := map[string]candb.Input{}
	for _, p := range pairs {
		name, val, ok := strings.Cut(p, "=")
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		if !ok || name == "" || val == "" {
			return nil, errs.Invalid("invalid assignment %q (want signal=value)", p)
		}
		if _, dup := out[name]; dup {
			return nil, errs.Invalid("signal %s given twice", name)
		}
		switch {
		case raw:
			if v, err := strconv.ParseUint(val, 0, 64); err == nil {
				out[name] = candb.Raw(v)
			} else if v, err := strconv.ParseInt(val, 0, 64); err == nil {
				out[name] = candb.RawInt(v)
			} else {
				return nil, errs.Invalid("invalid raw value %q for %s (an integer)", val, name)
			}
		default:
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				out[name] = candb.Phys(v)
			} else {
				out[name] = candb.Label(strings.Trim(val, `"`))
			}
		}
	}
	return out, nil
}

// LoadDatabase reads a DBC or KCD file (by extension, or by content
// starting with '<'). Diagnostics are returned also when err is set.
func LoadDatabase(path string) (*candb.Database, []candb.Diagnostic, string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot read database: %w", err)
	}
	kind := "dbc"
	if strings.EqualFold(filepath.Ext(path), ".kcd") || bytes.HasPrefix(bytes.TrimSpace(src), []byte("<")) {
		kind = "kcd"
	}
	var db *candb.Database
	var diags []candb.Diagnostic
	if kind == "kcd" {
		db, diags, err = kcd.Parse(bytes.NewReader(src))
	} else {
		db, diags, err = dbc.ParseBytes(src)
	}
	if err != nil {
		return db, diags, kind, &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidDatabase,
			Message: fmt.Sprintf("%s: %v", path, err), Cause: err}
	}
	return db, diags, kind, nil
}

// OpenTrace opens a capture file ("-" for stdin) of any supported format.
func OpenTrace(path string, stdin io.Reader) (capture.Reader, func(), error) {
	r := stdin
	closer := func() {}
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot open trace: %w", err)
		}
		r, closer = f, func() { _ = f.Close() }
	}
	cr, _, err := capture.NewReader(r)
	if err != nil {
		closer()
		return nil, nil, &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture, Message: fmt.Sprintf("%s: %v", path, err), Cause: err}
	}
	return cr, closer, nil
}

// TraceFormat picks a capture format from a file extension, or from an
// explicit name.
func TraceFormat(path, explicit string) (capture.Format, error) {
	if explicit != "" {
		switch capture.Format(explicit) {
		case capture.FormatCandump, capture.FormatJSONL, capture.FormatPCAP:
			return capture.Format(explicit), nil
		default:
		}
		return "", errs.Invalid("unknown trace format %q (candump, jsonl or pcap)", explicit)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jsonl", ".json":
		return capture.FormatJSONL, nil
	case ".pcap":
		return capture.FormatPCAP, nil
	}
	return capture.FormatCandump, nil
}

// ReadLines reads non-empty lines (for frames given on stdin).
func ReadLines(r io.Reader) ([]string, error) {
	var out []string
	s := bufio.NewScanner(r)
	for s.Scan() {
		if l := strings.TrimSpace(s.Text()); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, s.Err()
}

// TransmitCheck enforces the transmit safety rule: on an interface that
// is not virtual (canbusctl's in-process bus, vcan or vxcan), every
// transmission needs --yes.
func TransmitCheck(iface string, yes bool, what string) error {
	if iface == Virtual || yes {
		return nil
	}
	it, err := link.Get(iface)
	if err == nil && it.Virtual() {
		return nil
	}
	kind := "a physical or unknown interface"
	if err == nil {
		kind = "a " + it.Kind + " interface"
	}
	if errors.Is(err, canbus.ErrInterfaceDown) {
		return errs.New(errs.KindTransport, errs.CodeInterface, "%s: %v", iface, err)
	}
	if errors.Is(err, canbus.ErrUnsupported) && !strings.Contains(err.Error(), "not a CAN interface") {
		return errs.New(errs.KindUnsupported, errs.CodeUnsupported, "%s: %v", iface, err)
	}
	return errs.New(errs.KindSafety, errs.CodeNeedsYes,
		"%s would transmit on %s, %s: CAN frames can move actuators. Check the bus, then repeat with --yes", what, iface, kind)
}

// FrameJSON is the JSON form of a frame in canbusctl output.
type FrameJSON struct {
	Timestamp string `json:"timestamp,omitempty"`
	Interface string `json:"interface,omitempty"`
	ID        uint32 `json:"id"`
	IDHex     string `json:"idHex"`
	Extended  bool   `json:"extended"`
	FD        bool   `json:"fd"`
	BRS       bool   `json:"brs"`
	ESI       bool   `json:"esi"`
	Remote    bool   `json:"remote"`
	Error     bool   `json:"error"`
	Len       int    `json:"len"`
	Data      string `json:"data"`
	Echo      bool   `json:"echo,omitempty"`
}

// ToJSON converts a frame.
func ToJSON(f *canbus.Frame) FrameJSON {
	j := FrameJSON{Interface: f.Interface, ID: uint32(f.ID), Extended: f.IsExtended(), FD: f.IsFD(),
		BRS: f.Flags&canbus.FlagBRS != 0, ESI: f.Flags&canbus.FlagESI != 0, Remote: f.IsRemote(), Error: f.IsError(),
		Len: int(f.Len), Data: hex.EncodeToString(f.Payload()), Echo: f.Echo}
	j.IDHex = fmt.Sprintf("%03X", j.ID)
	if j.Extended || j.Error {
		j.IDHex = fmt.Sprintf("%08X", j.ID)
	}
	if !f.Timestamp.IsZero() {
		j.Timestamp = Timestamp(f.Timestamp)
	}
	return j
}

// Timestamp formats a time as seconds with microseconds, as candump.
func Timestamp(t time.Time) string {
	return fmt.Sprintf("%d.%06d", t.Unix(), t.Nanosecond()/1000)
}

// FrameLine is the text form of a received frame (candump -L style).
func FrameLine(f *canbus.Frame) string {
	iface := f.Interface
	if iface == "" {
		iface = "-"
	}
	ts := "(-)"
	if !f.Timestamp.IsZero() {
		ts = "(" + Timestamp(f.Timestamp) + ")"
	}
	s := ts + " " + iface + " " + f.String()
	if f.IsError() {
		if info, ok := canbus.ParseError(f); ok {
			s += "  ERROR " + info.String()
		}
	}
	return s
}

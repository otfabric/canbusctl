// SPDX-License-Identifier: MIT

// Package cli holds what several canbusctl commands share: parsing of
// identifiers, frames and signal assignments, loading databases and
// traces, opening interfaces and the transmit safety check.
package cli

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/canbusctl/internal/output"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/arxml"
	"github.com/otfabric/go-canbus/candb"
	"github.com/otfabric/go-canbus/capture"
	"github.com/otfabric/go-canbus/dbc"
	"github.com/otfabric/go-canbus/kcd"
	"github.com/otfabric/go-canbus/socketcan/link"
)

// Virtual is the interface name of canbusctl's in-process virtual bus.
const Virtual = "virtual"

// SlcanPrefix starts an interface that is a serial-line CAN adapter:
// "slcan:/dev/ttyACM0", "slcan:COM3" or "slcan:tcp://host:port".
const SlcanPrefix = "slcan:"

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
// (physical, kept as decimal text so 64-bit values stay exact; raw with
// raw), or a value description.
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
			if _, err := strconv.ParseFloat(val, 64); err == nil {
				// Decimal text, not a float64: a 64-bit raw value of a
				// factor-1 signal stays exact.
				out[name] = candb.PhysText(val)
			} else {
				out[name] = candb.Label(strings.Trim(val, `"`))
			}
		}
	}
	return out, nil
}

// MaxDatabaseHead bounds the white space read before the content of a
// database file without a .dbc, .kcd or .arxml extension is recognised: the
// largest source go-canbus parses (ARXML, 1 GiB; DBC and KCD 64 MiB).
const MaxDatabaseHead = 1 << 30

// LoadDatabase reads a DBC, KCD or ARXML file (by extension, or by
// content: see DatabaseKind). The file is streamed to the go-canbus
// parser, which bounds it (DBC and KCD 64 MiB, ARXML 1 GiB): a larger file,
// a FIFO or a device is refused once the bound is read, not read whole.
// Diagnostics are returned also when err is set.
func LoadDatabase(path string) (*candb.Database, []candb.Diagnostic, string, error) {
	return loadDatabase(path, nil)
}

// LoadDatabaseSum is LoadDatabase that also returns the SHA-256 of the
// whole file, read once.
func LoadDatabaseSum(path string) (*candb.Database, []candb.Diagnostic, string, [sha256.Size]byte, error) {
	h := sha256.New()
	db, diags, kind, err := loadDatabase(path, h)
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return db, diags, kind, sum, err
}

// readErr remembers the first error of r other than io.EOF.
type readErr struct {
	r   io.Reader
	err error
}

func (e *readErr) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) && e.err == nil {
		e.err = err
	}
	return n, err
}

func loadDatabase(path string, sum io.Writer) (*candb.Database, []candb.Diagnostic, string, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, nil, "", errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot read database: %w", err)
	}
	defer func() { _ = f.Close() }()
	var r io.Reader = f
	if sum != nil {
		r = io.TeeReader(f, sum)
	}
	in := &readErr{r: r}
	readFailed := func() error {
		return errs.New(errs.KindInvalidInput, errs.CodeFileOpen, "cannot read database: %w", in.err)
	}
	br := bufio.NewReader(in)
	var head []byte
	kind := extensionKind(path)
	if kind == "" {
		head = readHead(br)
		if in.err != nil {
			return nil, nil, "", readFailed()
		}
		kind = DatabaseKind(path, head)
	}
	src := io.MultiReader(bytes.NewReader(head), br)
	var db *candb.Database
	var diags []candb.Diagnostic
	switch kind {
	case "kcd":
		db, diags, err = kcd.Parse(src)
	case "arxml":
		db, diags, err = arxml.Parse(src)
	default:
		db, diags, err = dbc.Parse(src)
	}
	if in.err != nil {
		return nil, nil, kind, readFailed()
	}
	if err != nil {
		return db, diags, kind, &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidDatabase,
			Message: fmt.Sprintf("%s: %v", path, err), Cause: err}
	}
	if sum != nil {
		// The parser may stop before the end (after the XML root): the
		// sum covers the whole file. Streamed, so the memory stays small.
		if _, err := io.Copy(io.Discard, br); err != nil || in.err != nil {
			return nil, nil, kind, readFailed()
		}
	}
	return db, diags, kind, nil
}

// readHead reads the leading white space of r and at most 4096 bytes
// after it: what DatabaseKind needs to recognise the content. It reads at
// most MaxDatabaseHead bytes of white space.
func readHead(r *bufio.Reader) []byte {
	var head []byte
	for len(head) < MaxDatabaseHead {
		b, _ := r.Peek(utf8.UTFMax)
		if len(b) == 0 {
			return head
		}
		c, n := utf8.DecodeRune(b)
		if !unicode.IsSpace(c) {
			break
		}
		head = append(head, b[:n]...)
		_, _ = r.Discard(n)
	}
	rest := make([]byte, 4096)
	n, _ := io.ReadFull(r, rest)
	return append(head, rest[:n]...)
}

// extensionKind is the kind of a database by its extension, "" when the
// extension does not tell.
func extensionKind(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".kcd":
		return "kcd"
	case ".arxml":
		return "arxml"
	case ".dbc":
		return "dbc"
	}
	return ""
}

// DatabaseKind tells DBC, KCD and ARXML apart: by extension, else by
// content (XML with an AUTOSAR root is ARXML, other XML is KCD).
func DatabaseKind(path string, src []byte) string {
	if k := extensionKind(path); k != "" {
		return k
	}
	trim := bytes.TrimSpace(src)
	if bytes.HasPrefix(trim, []byte("<")) {
		head := trim[:min(len(trim), 4096)]
		if bytes.Contains(head, []byte("<AUTOSAR")) {
			return "arxml"
		}
		return "kcd"
	}
	return "dbc"
}

// OpenTrace opens a capture file ("-" for stdin) of any supported format.
func OpenTrace(path string, stdin io.Reader) (capture.Reader, func(), error) {
	r := stdin
	closer := func() {}
	if path != "-" {
		f, err := os.Open(filepath.Clean(path))
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
		case capture.FormatCandump, capture.FormatJSONL, capture.FormatPCAP, capture.FormatPCAPNG, capture.FormatASC, capture.FormatBLF, capture.FormatTRC:
			return capture.Format(explicit), nil
		default:
		}
		return "", errs.Invalid("unknown trace format %q (candump, jsonl, pcap, pcapng, asc, blf or trc)", explicit)
	}
	switch f := capture.FormatFromPath(path); f {
	case "", capture.FormatCandump:
		return capture.FormatCandump, nil
	default:
		return f, nil
	}
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
	if strings.HasPrefix(iface, SlcanPrefix) {
		return errs.New(errs.KindSafety, errs.CodeNeedsYes,
			"%s would transmit on %s, a serial-line CAN adapter: CAN frames can move actuators. Check the bus, then repeat with --yes", what, iface)
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

// FrameLine is the text form of a received frame (candump -L style). The
// interface name, which a trace file may carry, is escaped (output.Escape).
func FrameLine(f *canbus.Frame) string {
	iface := output.Escape(f.Interface)
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

// OutputMode is the permission of files canbusctl creates (traces,
// generated code, converted databases): owner only, as captures can
// carry vehicle data.
const OutputMode = 0o600

// ErrOutputLink refuses an output path that is a symbolic link: writing
// through it would truncate whatever file the link points to.
var ErrOutputLink = errors.New("the output is a symbolic link; canbusctl does not write through links (name the target itself)")

// CreateOutput creates or truncates the file an operator named on the
// command line, owner-only. A symbolic link is refused (ErrOutputLink);
// one that appears between the check and the open fails the open
// (O_NOFOLLOW, where the platform has it). An existing regular file keeps
// its mode.
func CreateOutput(path string) (*os.File, error) {
	path = filepath.Clean(path)
	if OutputIsLink(path) {
		return nil, ErrOutputLink
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|oNoFollow, OutputMode)
}

// WriteOutput writes data to a file created by CreateOutput.
func WriteOutput(path string, data []byte) error {
	f, err := CreateOutput(path)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}

// SameFile reports that the paths name one existing file (a link to it
// included): writing the output would truncate the input being read.
func SameFile(a, b string) bool {
	sa, err := os.Stat(filepath.Clean(a))
	if err != nil {
		return false
	}
	sb, err := os.Stat(filepath.Clean(b))
	return err == nil && os.SameFile(sa, sb)
}

// RemovePartial removes an output file left incomplete by a failed
// command, when path itself is a regular file: never a device, a FIFO or
// the target of a symbolic link.
func RemovePartial(path string) {
	if st, err := os.Lstat(filepath.Clean(path)); err == nil && st.Mode().IsRegular() {
		_ = os.Remove(filepath.Clean(path))
	}
}

// CheckTrace reads a whole trace file and reports its first malformed
// record (invalid_capture), so that a transmitting command can refuse a
// bad file before it sends anything. Standard input ("-") cannot be read
// twice and is not checked.
func CheckTrace(path string) error {
	if path == "-" {
		return nil
	}
	r, closer, err := OpenTrace(path, nil)
	if err != nil {
		return err
	}
	defer closer()
	xr, _ := r.(capture.XLReader)
	var f canbus.Frame
	var x canbus.XLFrame
	for n := 1; ; n++ {
		if xr != nil {
			_, err = xr.ReadAny(&f, &x)
		} else {
			err = r.Read(&f)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return &errs.Error{Kind: errs.KindInvalidInput, Code: errs.CodeInvalidCapture,
				Message: fmt.Sprintf("%s: %v (nothing sent)", path, err), Cause: err}
		}
	}
}

// OutputIsLink reports that path exists as a symbolic link.
func OutputIsLink(path string) bool {
	st, err := os.Lstat(filepath.Clean(path))
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type res struct{ N int }

func (r res) Text() string      { return "n=1\n" }
func (r res) Headers() []string { return []string{"A", "B"} }
func (r res) Rows() [][]string  { return [][]string{{"1", "22"}} }

type plain struct{}

func (plain) Text() string { return "plain" }

func TestFormats(t *testing.T) {
	for in, want := range map[string]Format{"": Text, "JSON": JSON, " table ": Table} {
		if f, err := Parse(in); err != nil || f != want {
			t.Fatal(in, f, err)
		}
	}
	if _, err := Parse("xml"); err == nil {
		t.Fatal("xml")
	}
	var b bytes.Buffer
	_ = Write(&b, Text, "c", res{})
	_ = Write(&b, Table, "c", res{})
	_ = Write(&b, Table, "c", plain{})
	_ = Write(&b, JSON, "c", res{N: 2})
	out := b.String()
	for _, w := range []string{"n=1\nA  B\n1  22\nplain\n", `"schemaVersion": "1.0"`, `"command": "c"`, `"N": 2`} {
		if !strings.Contains(out, w) {
			t.Fatalf("lacks %q:\n%s", w, out)
		}
	}
	b.Reset()
	_ = WriteError(&b, map[string]string{"kind": "x"})
	if !strings.Contains(b.String(), `"error"`) {
		t.Fatal(b.String())
	}
	if err := Write(&b, JSON, "c", bad{}); err == nil {
		t.Fatal("unencodable value")
	}
}

type bad struct{ C chan int }

func (bad) Text() string { return "" }

func TestStream(t *testing.T) {
	var b bytes.Buffer
	s := NewStream(&b, JSON)
	_ = s.Record("frame", "ignored", map[string]int{"id": 1})
	_ = s.Record("empty", "", struct{}{})
	if b.String() != `{"schemaVersion":"1.0","type":"frame","id":1}`+"\n"+`{"schemaVersion":"1.0","type":"empty"}`+"\n" {
		t.Fatal(b.String())
	}
	if err := s.Record("x", "", make(chan int)); err == nil {
		t.Fatal("unencodable")
	}
	b.Reset()
	_ = NewStream(&b, Text).Record("frame", "line", nil)
	if b.String() != "line\n" {
		t.Fatal(b.String())
	}
	_ = errors.New
}

// u writes the escape U+hhhh of a test case as Escape does (\u then four
// hexadecimal digits).
func u(s string) string { return strings.ReplaceAll(s, "U+", `\`+"u") }

// TestEscape: the characters a terminal acts on are written as escapes in
// text and table output: C0 controls and DEL as \x.., C1 controls and the
// bidirectional formatting characters as \u...., bytes that are not
// UTF-8 as \x..; printable text, other Unicode and the line breaks and
// tabs of a text layout pass unchanged. They were written as they came.
func TestEscape(t *testing.T) {
	for _, c := range []struct{ in, esc, text string }{
		{"", "", ""},
		{"Engine Speed", "Engine Speed", "Engine Speed"},
		{"\U000000b0C \U000000b5s \U000003a9 \U00006e29\U00005ea6", "\U000000b0C \U000000b5s \U000003a9 \U00006e29\U00005ea6", "\U000000b0C \U000000b5s \U000003a9 \U00006e29\U00005ea6"},
		{"\x1b[2J", `\x1b[2J`, `\x1b[2J`},
		{"a\x00b\x07c\x08d\x7f", `a\x00b\x07c\x08d\x7f`, `a\x00b\x07c\x08d\x7f`},
		{"\x1f \x20 \x7e", `\x1f ` + "\x20 \x7e", `\x1f ` + "\x20 \x7e"},
		{"line\nnext\ttab\r", `line\x0anext\x09tab\x0d`, "line\nnext\ttab" + `\x0d`},
		{"\U00000080\U0000009b\U0000009f\U000000a0", u("U+0080U+009bU+009f") + "\U000000a0", u("U+0080U+009bU+009f") + "\U000000a0"},
		{"abc\U0000202edef", u("abcU+202edef"), u("abcU+202edef")},
		{"\U0000061c\U0000200e\U0000200f\U0000202a\U0000202b\U0000202c\U0000202d\U00002066\U00002067\U00002068\U00002069",
			u("U+061cU+200eU+200fU+202aU+202bU+202cU+202dU+2066U+2067U+2068U+2069"),
			u("U+061cU+200eU+200fU+202aU+202bU+202cU+202dU+2066U+2067U+2068U+2069")},
		{"\U0000200d\U00002065\U0000206a\U00002029", "\U0000200d\U00002065\U0000206a\U00002029", "\U0000200d\U00002065\U0000206a\U00002029"},
		{"bad\xff\xfe end", `bad\xff\xfe end`, `bad\xff\xfe end`},
		{"cut \xe2\x80", `cut \xe2\x80`, `cut \xe2\x80`},
	} {
		if got := Escape(c.in); got != c.esc {
			t.Errorf("Escape(%q) = %q, want %q", c.in, got, c.esc)
		}
		if got := escapeText(c.in); got != c.text {
			t.Errorf("escapeText(%q) = %q, want %q", c.in, got, c.text)
		}
	}
}

type ctlResult struct{ s string }

func (r ctlResult) Text() string      { return r.s }
func (r ctlResult) Headers() []string { return []string{"NAME\x1b", "N"} }
func (r ctlResult) Rows() [][]string  { return [][]string{{r.s, "1"}} }

// TestEscapedOutput: text results, table cells and text records are
// escaped; JSON carries the string itself, escaped by JSON.
func TestEscapedOutput(t *testing.T) {
	const s = "x\x1b]0;t\x07\U0000202ey"
	escaped := u(`x\x1b]0;t\x07U+202ey`)
	clean := func(what, out string) {
		t.Helper()
		for _, r := range out {
			if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f || r >= 0x80 && r <= 0x9f || r == 0x202e {
				t.Fatalf("%s: %U written: %q", what, r, out)
			}
		}
	}
	for _, f := range []Format{Text, Table} {
		var b bytes.Buffer
		if err := Write(&b, f, "c", ctlResult{s}); err != nil {
			t.Fatal(err)
		}
		clean(string(f), b.String())
		if !strings.Contains(b.String(), escaped) {
			t.Fatalf("%s: %q", f, b.String())
		}
	}
	var b bytes.Buffer
	if err := NewStream(&b, Text).Record("frame", "a\t"+s+"\nb", nil); err != nil {
		t.Fatal(err)
	}
	clean("record", b.String())
	if b.String() != "a\t"+escaped+"\nb\n" {
		t.Fatalf("record %q", b.String())
	}
	b.Reset()
	if err := Write(&b, JSON, "c", ctlJSON{s}); err != nil {
		t.Fatal(err)
	}
	var doc struct{ Result ctlJSON }
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil || doc.Result.Name != s {
		t.Fatalf("JSON %q: %v", b.String(), err)
	}
}

type ctlJSON struct {
	Name string `json:"name"`
}

func (r ctlJSON) Text() string { return r.Name }

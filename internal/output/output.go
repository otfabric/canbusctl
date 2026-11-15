// SPDX-License-Identifier: MIT

// Package output renders results as text, tables or versioned JSON
// (docs/json-output.md).
package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// SchemaVersion of every JSON document canbusctl writes.
const SchemaVersion = "1.0"

// Format is an output format.
type Format string

// Formats.
const (
	Text  Format = "text"
	JSON  Format = "json"
	Table Format = "table"
)

// Parse validates a --format value ("" is text).
func Parse(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case "":
		return Text, nil
	case Text, JSON, Table:
		return f, nil
	}
	return "", fmt.Errorf("invalid --format %q (text, json or table)", s)
}

// Result is a command result.
type Result interface {
	// Text renders the human-readable form.
	Text() string
}

// Tabular results can also render as a table.
type Tabular interface {
	Headers() []string
	Rows() [][]string
}

// Envelope is the JSON document of a result.
type Envelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Command       string `json:"command"`
	Result        any    `json:"result"`
}

// ErrorEnvelope is the JSON document of a fatal error.
type ErrorEnvelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Error         any    `json:"error"`
}

// Write renders r.
func Write(w io.Writer, f Format, command string, r Result) error {
	switch f {
	case JSON:
		return writeJSON(w, Envelope{SchemaVersion, command, r})
	case Table:
		if t, ok := r.(Tabular); ok {
			return WriteTable(w, t.Headers(), t.Rows())
		}
	default:
	}
	s := escapeText(strings.TrimSuffix(r.Text(), "\n"))
	_, err := io.WriteString(w, s+"\n")
	return err
}

// Escape returns s with the characters a terminal acts on written as
// escapes: the C0 control characters (\n and \t included) and DEL as
// \x1b, the C1 control characters as \u009b, the Unicode bidirectional
// formatting characters as \u202e, and bytes that are not UTF-8 as \xff.
// Text and table output pass the strings of databases and traces through
// it; JSON escapes them itself.
func Escape(s string) string { return escape(s, false) }

// escapeText is Escape for a whole text result or record: it keeps the
// line breaks and tabs of its layout.
func escapeText(s string) string { return escape(s, true) }

func escape(s string, layout bool) string {
	var b *strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x20 && c < 0x7f {
			if b != nil {
				b.WriteByte(c)
			}
			i++
			continue
		}
		r, n := rune(c), 1
		if c >= utf8.RuneSelf {
			r, n = utf8.DecodeRuneInString(s[i:])
		}
		var esc string
		switch {
		case r == utf8.RuneError && n == 1:
			esc = fmt.Sprintf(`\x%02x`, c)
		case r < 0x20 || r == 0x7f:
			if !layout || r != '\n' && r != '\t' {
				esc = fmt.Sprintf(`\x%02x`, r)
			}
		case r >= 0x80 && r <= 0x9f || bidi(r):
			esc = fmt.Sprintf(`\u%04x`, r)
		}
		if esc == "" {
			if b != nil {
				b.WriteString(s[i : i+n])
			}
			i += n
			continue
		}
		if b == nil {
			b = &strings.Builder{}
			b.Grow(len(s) + 8)
			b.WriteString(s[:i])
		}
		b.WriteString(esc)
		i += n
	}
	if b == nil {
		return s
	}
	return b.String()
}

// bidi reports a Unicode bidirectional formatting character: the marks
// ALM, LRM and RLM, the embeddings and overrides LRE..RLO and the isolates
// LRI..PDI.
func bidi(r rune) bool {
	return r == 0x061C || r == 0x200E || r == 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069
}

// WriteError renders a fatal error document.
func WriteError(w io.Writer, e any) error {
	return writeJSON(w, ErrorEnvelope{SchemaVersion, e})
}

func writeJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// WriteTable writes aligned columns. Every cell is escaped (Escape).
func WriteTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, joinCells(headers))
	for _, r := range rows {
		_, _ = fmt.Fprintln(tw, joinCells(r))
	}
	return tw.Flush()
}

func joinCells(cells []string) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = Escape(c)
	}
	return strings.Join(out, "\t")
}

// Stream writes one record per line: a line of text, or with JSON one
// compact JSON object per line (JSON Lines) carrying schemaVersion and
// type.
type Stream struct {
	w io.Writer
	f Format
}

// NewStream returns a record stream.
func NewStream(w io.Writer, f Format) *Stream { return &Stream{w: w, f: f} }

// Record writes one record: text for text and table output, typ and
// fields for JSON.
func (s *Stream) Record(typ string, text string, fields any) error {
	if s.f != JSON {
		_, err := io.WriteString(s.w, escapeText(text)+"\n")
		return err
	}
	b, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	// Splice schemaVersion and type in front of the record's own fields.
	head := fmt.Sprintf(`{"schemaVersion":%q,"type":%q`, SchemaVersion, typ)
	body := strings.TrimPrefix(strings.TrimSpace(string(b)), "{")
	if body == "}" {
		_, err = io.WriteString(s.w, head+"}\n")
	} else {
		_, err = io.WriteString(s.w, head+","+body+"\n")
	}
	return err
}

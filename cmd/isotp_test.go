// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// pairRun runs a receiving and a sending invocation on one virtual bus.
func pairRun(t *testing.T, recv, send []string) (result, result) {
	t.Helper()
	resetVirtual()
	_ = sharedVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rc := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := Run(ctx, recv, strings.NewReader(""), &out, &errOut)
		rc <- result{code, out.String(), errOut.String()}
	}()
	time.Sleep(150 * time.Millisecond)
	var out, errOut bytes.Buffer
	code := Run(ctx, send, strings.NewReader(""), &out, &errOut)
	return <-rc, result{code, out.String(), errOut.String()}
}

func TestISOTPExchange(t *testing.T) {
	common := []string{"-i", "virtual"}
	recv := append([]string{"isotp", "receive", "--tx-id", "7E8", "--rx-id", "7E0", "--bs", "4", "--stmin", "1ms", "-n", "2", "--timeout", "10s", "--format", "json", "--max-size", "5000"}, common...)
	send := append([]string{"isotp", "send", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "4100", "-n", "2", "--padding", "CC", "--max-size", "5000"}, common...)
	r, s := pairRun(t, recv, send)
	want(t, s, 0, "sent 2 message(s), 8200 bytes")
	want(t, r, 0)
	recs := jsonLines(t, r.out)
	if len(recs) != 2 || recs[0]["len"].(float64) != 4100 || recs[0]["type"] != "message" {
		t.Fatal(recs)
	}
	// Extended addressing, 29-bit identifiers and CAN FD.
	recv = append([]string{"isotp", "receive", "--tx-id", "18DAF110", "--rx-id", "18DA10F1", "--extended-ids", "--addressing", "extended",
		"--tx-ext", "F1", "--rx-ext", "10", "--fd", "--timeout", "10s"}, common...)
	send = append([]string{"isotp", "send", "--tx-id", "18DA10F1", "--rx-id", "18DAF110", "--extended-ids", "--addressing", "extended",
		"--tx-ext", "10", "--rx-ext", "F1", "--fd", "--tx-dl", "64", "--brs", "--data", "0102030405060708090A0B0C0D0E0F10"}, common...)
	r, s = pairRun(t, recv, send)
	want(t, s, 0, "sent 1")
	want(t, r, 0, "16 bytes: 0102030405060708090A0B0C0D0E0F10")
	// Monitor observes a transfer between two other endpoints.
	mon := append([]string{"isotp", "monitor", "--tx-id", "7E0", "--rx-id", "7E8", "--duration", "1s", "--flow-control"}, common...)
	resetVirtual()
	_ = sharedVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	mc := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := Run(ctx, mon, strings.NewReader(""), &out, &errOut)
		mc <- result{code, out.String(), errOut.String()}
	}()
	time.Sleep(100 * time.Millisecond)
	rc := make(chan int, 1)
	go func() {
		var o, e bytes.Buffer
		rc <- Run(ctx, append([]string{"isotp", "receive", "--tx-id", "7E8", "--rx-id", "7E0", "--timeout", "5s"}, common...), strings.NewReader(""), &o, &e)
	}()
	time.Sleep(100 * time.Millisecond)
	var o, e bytes.Buffer
	if code := Run(ctx, append([]string{"isotp", "send", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "100"}, common...), strings.NewReader(""), &o, &e); code != 0 {
		t.Fatal(o.String(), e.String())
	}
	if code := <-rc; code != 0 {
		t.Fatal("receiver", code)
	}
	m := <-mc
	want(t, m, 0, "7E0: 100 bytes", "flow control status 0")
}

func TestISOTPErrors(t *testing.T) {
	base := []string{"-i", "virtual"}
	for _, c := range []struct {
		args []string
		code int
		msg  string
	}{
		{[]string{"isotp", "send", "--size", "10"}, 3, "--tx-id and --rx-id"},
		{[]string{"isotp", "send", "--tx-id", "800", "--rx-id", "1", "--size", "1"}, 3, "11-bit"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "800", "--size", "1"}, 3, "11-bit"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2"}, 3, "--data"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--data", "01", "--size", "2"}, 3, "not both"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--data", "0"}, 3, "even"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "-n", "0"}, 3, "--count"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--addressing", "odd"}, 3, "--addressing"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--addressing", "mixed"}, 3, "--tx-ext"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--addressing", "mixed", "--tx-ext", "zz", "--rx-ext", "1"}, 3, "invalid byte"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--addressing", "mixed", "--tx-ext", "1", "--rx-ext", "zz"}, 3, "invalid byte"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--padding", "x"}, 3, "invalid byte"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--stmin", "2s"}, 3, "STmin"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "100", "--timeout", "200ms"}, 5, "deadline"},
		{[]string{"isotp", "send", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--kernel"}, 3, "--kernel needs"},
		{[]string{"isotp", "receive", "--tx-id", "1", "--rx-id", "2", "--timeout", "50ms"}, 5, "received 0 of 1"},
		{[]string{"isotp", "receive", "--tx-id", "1", "--rx-id", "2", "--timeout", "50ms", "-n", "0"}, 0, ""},
		{[]string{"isotp", "receive", "--rx-id", "2"}, 3, "--tx-id"},
		{[]string{"isotp", "monitor", "--tx-id", "1", "--rx-id", "2", "--kernel"}, 8, "--kernel"},
		{[]string{"isotp", "monitor", "--rx-id", "2"}, 3, "--tx-id"},
		{[]string{"isotp", "monitor", "--tx-id", "1", "--rx-id", "2", "--duration", "30ms"}, 0, ""},
	} {
		r := run(t, "", append(c.args, base...)...)
		want(t, r, c.code, c.msg)
	}
	if !linux() {
		want(t, run(t, "", "isotp", "send", "-i", "vcan0", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--kernel", "--yes"), 8)
	}
}

func TestPassiveReassembly(t *testing.T) {
	p := &passive{}
	steps := []struct {
		in   []byte
		msg  string
		note string
	}{
		{[]byte{0x03, 1, 2, 3}, "\x01\x02\x03", ""},
		{[]byte{0x00, 2, 9, 9}, "\x09\x09", ""}, // FD escape
		{[]byte{0x00}, "", "malformed single frame"},
		{[]byte{0x10, 0x09, 1, 2, 3, 4, 5, 6}, "", ""},
		{[]byte{0x30, 0, 0}, "", "flow control status 0"},
		{[]byte{0x21, 7, 8, 9}, "\x01\x02\x03\x04\x05\x06\x07\x08\x09", ""},
		{[]byte{0x21, 1}, "", ""}, // no transfer in progress
		{[]byte{0x10, 0x0A, 1, 2, 3, 4, 5, 6}, "", ""},
		{[]byte{0x23, 1}, "", "sequence number 3, expected 1"},
		{[]byte{0x10}, "", "malformed first frame"},
		{[]byte{0x10, 0x00, 0x7F, 0, 0, 0, 1, 2}, "", "announces"},
		{[]byte{0x10, 0x00, 0, 0, 0, 2, 7, 8}, "\x07\x08", ""},
		{nil, "", ""},
	}
	for i, s := range steps {
		msg, note := p.feed(s.in)
		if string(msg) != s.msg || !strings.Contains(note, s.note) || (s.note == "" && note != "") {
			t.Fatalf("step %d: %q %q", i, msg, note)
		}
	}
	e := &passive{off: 1}
	if msg, _ := e.feed([]byte{0xAA, 0x01, 0x42}); string(msg) != "\x42" {
		t.Fatal("extended addressing")
	}
}

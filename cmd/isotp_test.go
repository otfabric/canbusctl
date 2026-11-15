// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

// background runs a command in its own goroutine; its result arrives on
// the channel.
func background(ctx context.Context, args []string) chan result {
	rc := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := Run(ctx, args, strings.NewReader(""), &out, &errOut)
		rc <- result{code, out.String(), errOut.String()}
	}()
	return rc
}

// pairRun runs a receiving and a sending invocation on one virtual bus,
// the sender once the receiver listens.
func pairRun(t *testing.T, recv, send []string) (result, result) {
	t.Helper()
	resetVirtual()
	_ = sharedVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ready := readySignals(t, "isotp receive")
	rc := background(ctx, recv)
	await(t, "isotp receive", ready["isotp receive"], rc)
	var out, errOut bytes.Buffer
	code := Run(ctx, send, strings.NewReader(""), &out, &errOut)
	return <-rc, result{code, out.String(), errOut.String()}
}

func TestISOTPExchange(t *testing.T) {
	common := []string{"-i", "virtual"}
	recv := append([]string{"isotp", "receive", "--tx-id", "7E8", "--rx-id", "7E0", "--bs", "4", "--stmin", "1ms", "--fc-delay", "200us", "-n", "2", "--timeout", "10s", "--format", "json", "--max-size", "5000"}, common...)
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
	mon := append([]string{"isotp", "monitor", "--tx-id", "7E0", "--rx-id", "7E8", "--duration", "1s"}, common...)
	resetVirtual()
	_ = sharedVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ready := readySignals(t, "isotp monitor", "isotp receive")
	mc := background(ctx, mon)
	await(t, "isotp monitor", ready["isotp monitor"], mc)
	rc := background(ctx, append([]string{"isotp", "receive", "--tx-id", "7E8", "--rx-id", "7E0", "--timeout", "5s"}, common...))
	await(t, "isotp receive", ready["isotp receive"], rc)
	var o, e bytes.Buffer
	if code := Run(ctx, append([]string{"isotp", "send", "--tx-id", "7E0", "--rx-id", "7E8", "--size", "100"}, common...), strings.NewReader(""), &o, &e); code != 0 {
		t.Fatal(o.String(), e.String())
	}
	if r := <-rc; r.code != 0 || !strings.Contains(r.out, "100 bytes") {
		t.Fatal("receiver", r)
	}
	m := <-mc
	want(t, m, 0, "7E0: 100 bytes: 00010203")
	if strings.Contains(m.out, "7E8:") {
		t.Fatal("the flow control frames were taken for a message")
	}
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
		{[]string{"isotp", "receive", "--tx-id", "1", "--rx-id", "2", "--fc-delay", "2s"}, 3, "FlowControlDelay"},
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
	want(t, run(t, "", "isotp", "receive", "-i", "vcan0", "--tx-id", "1", "--rx-id", "2", "--kernel", "--fc-delay", "1ms"), 8, "--fc-delay")
	if !linux() {
		want(t, run(t, "", "isotp", "send", "-i", "vcan0", "--tx-id", "1", "--rx-id", "2", "--size", "1", "--kernel", "--yes"), 8)
	}
}

func TestISOTPMonitorNotes(t *testing.T) {
	// A first frame followed by a consecutive frame with a wrong sequence
	// number: the monitor reports it and never transmits.
	resetVirtual()
	vb := sharedVirtual()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ready := readySignals(t, "isotp monitor")
	mc := background(ctx, []string{"isotp", "monitor", "-i", "virtual", "--tx-id", "7E0", "--rx-id", "7E8", "--duration", "500ms", "--format", "json"})
	await(t, "isotp monitor", ready["isotp monitor"], mc)
	spy, _ := vb.Open()
	tx, _ := vb.Open()
	for _, s := range []string{"7E0#1014000102030405", "7E0#2306070809"} {
		f, _ := parseForTest(s)
		_ = tx.Send(ctx, &f)
	}
	m := <-mc
	want(t, m, 0, `"type":"note"`, "sequence")
	c2, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stop()
	for {
		var f canbusFrame
		if spy.Receive(c2, &f) != nil {
			break
		}
		if f.ID == 0x7E8 {
			t.Fatal("the monitor transmitted", f)
		}
	}
}

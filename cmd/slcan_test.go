// SPDX-License-Identifier: MIT

package cmd

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/slcan"
)

// adapter is a TCP server that plays a serial-line CAN adapter: it
// records the commands canbusctl sends and exchanges frames with it.
type adapter struct {
	ln    net.Listener
	conns chan net.Conn
}

func newAdapter(t *testing.T) *adapter {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a := &adapter{ln: ln, conns: make(chan net.Conn, 4)}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			a.conns <- c
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return a
}

func (a *adapter) iface() string { return "slcan:tcp://" + a.ln.Addr().String() }

// accept waits for canbusctl to connect and returns its setup commands
// (up to "O" or "L") and a Conn for the frames that follow.
func (a *adapter) accept(t *testing.T) ([]string, *slcan.Conn) {
	t.Helper()
	var c net.Conn
	select {
	case c = <-a.conns:
	case <-time.After(10 * time.Second):
		t.Fatal("canbusctl did not connect")
	}
	r := bufio.NewReader(c)
	var cmds []string
	for {
		_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
		s, err := r.ReadString('\r')
		if err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, strings.TrimSuffix(s, "\r"))
		if cmds[len(cmds)-1] == "O" || cmds[len(cmds)-1] == "L" {
			break
		}
	}
	_ = c.SetReadDeadline(time.Time{})
	conn, err := slcan.New(bufConn{r, c}, slcan.Config{NoCommands: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return cmds, conn
}

// bufConn reads through the reader that took the setup commands.
type bufConn struct {
	r *bufio.Reader
	net.Conn
}

func (b bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }

func TestSlcanMonitorListenOnly(t *testing.T) {
	ad := newAdapter(t)
	done := make(chan result, 1)
	go func() {
		done <- run(t, "", "monitor", "-i", ad.iface(), "-n", "2", "--filter", "123:7FF", "--duration", "10s", "--slcan-bitrate", "250000")
	}()
	cmds, conn := ad.accept(t)
	if got := strings.Join(cmds, " "); got != "C S5 L" {
		t.Fatalf("setup %q: a passive command must open listen-only", got)
	}
	ctx := context.Background()
	for _, s := range []string{"t1232A1B2", "t4561FF", "t1230", "T18DAF1101AA"} {
		var f canbus.Frame
		_, _ = slcan.ParseFrame([]byte(s), &f)
		_ = conn.Send(ctx, &f)
	}
	r := <-done
	// The filter runs in software: 456 is not shown.
	want(t, r, 0, "123#A1B2", "123#")
	if strings.Contains(r.out, "456#") || strings.Contains(r.out, "18DAF110") {
		t.Fatal(r.out)
	}
}

func TestSlcanSendNeedsYes(t *testing.T) {
	ad := newAdapter(t)
	r := run(t, "", "send", "-i", ad.iface(), "123#01")
	want(t, r, 9)
	if !strings.Contains(r.errOut, "serial-line CAN adapter") {
		t.Fatal(r.errOut)
	}
	done := make(chan result, 1)
	go func() {
		done <- run(t, "", "send-fd", "-i", ad.iface(), "--yes", "--slcan-bitrate", "1000000", "--slcan-data-bitrate", "2000000", "7FF##1"+strings.Repeat("AB", 12))
	}()
	cmds, conn := ad.accept(t)
	if got := strings.Join(cmds, " "); got != "C S8 Y2 O" {
		t.Fatalf("setup %q", got)
	}
	var f canbus.Frame
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := conn.Receive(ctx, &f); err != nil || f.String() != "7FF##1"+strings.Repeat("AB", 12) {
		t.Fatal(f, err)
	}
	want(t, <-done, 0)
}

func TestSlcanErrors(t *testing.T) {
	want(t, run(t, "", "monitor", "-i", "slcan:tcp://127.0.0.1:1", "--duration", "1s"), 4)
	want(t, run(t, "", "monitor", "-i", "slcan:/nonexistent/tty", "--duration", "1s"), 4)
	want(t, run(t, "", "monitor", "-i", "virtual", "--slcan-bitrate", "42", "--duration", "1s"), 3)
	want(t, run(t, "", "monitor", "-i", "virtual", "--slcan-data-bitrate", "4000000", "--duration", "1s"), 3)
}

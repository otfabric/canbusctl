// SPDX-License-Identifier: MIT

//go:build linux

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/otfabric/go-canbus/socketcan/link"
)

const (
	nSLCAN      = 17     // N_SLCAN line discipline
	siocgifname = 0x8910 // SIOCGIFNAME on a tty: the netdev of its line discipline
)

// kernelSlcan links two pseudo-terminals with socat and attaches the Linux
// slcan driver to one: the kernel turns it into a slcanN interface, and
// the other tty is what a USB adapter's would be. It returns the
// interface and the tty path. Needs root, socat and the slcan module:
// skipped without CANBUS_TEST_ADMIN=1, failed without the module when
// CANBUS_REQUIRE_SLCAN=1.
func kernelSlcan(t *testing.T) (string, string) {
	t.Helper()
	if os.Getenv("CANBUS_TEST_ADMIN") != "1" {
		t.Skip("set CANBUS_TEST_ADMIN=1 and run as root")
	}
	socat, err := exec.LookPath("socat")
	if err != nil {
		if os.Getenv("CANBUS_REQUIRE_SLCAN") == "1" {
			t.Fatal("socat is not installed")
		}
		t.Skip("socat is not installed")
	}
	dir := t.TempDir()
	a, b := filepath.Join(dir, "kernel"), filepath.Join(dir, "adapter")
	cmd := exec.Command(socat, "PTY,raw,echo=0,link="+a, "PTY,raw,echo=0,link="+b)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(b); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	tty, err := os.OpenFile(a, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tty.Close() }) // removes the interface
	ld := int32(nSLCAN)
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCSETD, uintptr(unsafe.Pointer(&ld))); e != 0 {
		if os.Getenv("CANBUS_REQUIRE_SLCAN") == "1" {
			t.Fatalf("TIOCSETD N_SLCAN: %v", e)
		}
		t.Skipf("no slcan line discipline: %v (modprobe slcan)", e)
	}
	var name [16]byte
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), siocgifname, uintptr(unsafe.Pointer(&name[0]))); e != 0 {
		t.Fatalf("SIOCGIFNAME: %v", e)
	}
	ifc := string(name[:bytes.IndexByte(name[:], 0)])
	if err := link.SetUp(ifc); err != nil {
		t.Fatal(err)
	}
	return ifc, b
}

// startOnTTY starts canbusctl and waits until it has opened tty (an slcan
// adapter has no CAN socket to wait for).
func startOnTTY(t *testing.T, tty string, args ...string) *proc {
	t.Helper()
	real, err := filepath.EvalSymlinks(tty)
	if err != nil {
		t.Fatal(err)
	}
	p := &proc{done: make(chan struct{}), lists: -1}
	p.cmd = exec.Command(bin, args...)
	p.cmd.Stdout, p.cmd.Stderr = &p.o, &p.e
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		if ee, ok := p.cmd.Wait().(*exec.ExitError); ok {
			p.code = ee.ExitCode()
		}
		close(p.done)
	}()
	t.Cleanup(func() {
		select {
		case <-p.done:
		default:
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	dir := fmt.Sprintf("/proc/%d/fd", p.cmd.Process.Pid)
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if l, err := os.Readlink(filepath.Join(dir, e.Name())); err == nil && l == real {
				time.Sleep(100 * time.Millisecond) // the setup commands
				return p
			}
		}
		select {
		case <-p.done:
			return p
		default:
		}
	}
	t.Fatal("canbusctl did not open " + tty)
	return nil
}

// TestSlcanKernel: canbusctl drives an slcan "adapter" whose other end is
// the Linux kernel's slcan driver, and checks the frames on the slcanN
// interface the kernel creates — both directions.
func TestSlcanKernel(t *testing.T) {
	ifc, tty := kernelSlcan(t)
	frames := []string{"123#A1B2", "18DAF110#0210030000000000", "7DF#R"}
	// canbusctl → adapter tty → kernel → SocketCAN.
	mon := start(t, "monitor", "-i", ifc, "-n", "3", "--duration", "20s")
	want(t, run(t, "", append([]string{"send", "-i", "slcan:" + tty, "--yes"}, frames...)...), 0)
	r := mon.wait(t, 30*time.Second)
	want(t, r, 0, "123#A1B2", "18DAF110#0210030000000000", "7DF#R")
	// SocketCAN → kernel → adapter tty → canbusctl (listen-only).
	rec := filepath.Join(t.TempDir(), "rx.trc")
	p := startOnTTY(t, tty, "record", "-i", "slcan:"+tty, "-o", rec, "-n", "3", "--duration", "20s")
	want(t, run(t, "", append([]string{"send", "-i", ifc, "--yes"}, frames...)...), 0)
	want(t, p.wait(t, 30*time.Second), 0, "recorded 3 frames")
	got := run(t, "", "trace", "info", rec)
	want(t, got, 0, "3 frames")
	if !strings.Contains(got.out, "remote 1") {
		t.Fatalf("the remote frame did not survive:\n%s", got.out)
	}
}

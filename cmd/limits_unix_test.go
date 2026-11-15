// SPDX-License-Identifier: MIT

//go:build unix

package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestDatabaseFIFO: a database that is a FIFO whose writer sends more than
// the DBC bound (64 MiB) and keeps it open is refused once the bound is
// read. It was read to its end first, which never came.
func TestDatabaseFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pipe.dbc")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skip("no FIFO:", err)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		w, err := os.OpenFile(p, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer func() { _ = w.Close() }()
		_, _ = w.Write(bytes.Repeat([]byte("x"), 65<<20)) // fails once the reader closes
		<-done
	}()
	rc := make(chan result, 1)
	go func() { rc <- run(t, "", "dbc", "inspect", p, "--format", "json") }()
	select {
	case r := <-rc:
		want(t, r, 3, `"code": "invalid_database"`, "larger than 67108864")
	case <-time.After(60 * time.Second): // a failure bound, not a synchronisation
		t.Fatal("the database read did not end: the FIFO is read to its end")
	}
}

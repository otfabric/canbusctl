// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/slcan"
	"github.com/otfabric/go-canbus/socketcan"
	"github.com/otfabric/go-canbus/socketcan/link"
	"github.com/otfabric/go-canbus/virtual"
)

// openOpts are what a command asks of a transport.
type openOpts struct {
	fd      bool // CAN FD wanted (always on for the virtual bus)
	autoFD  bool // CAN FD if the interface can carry it
	filters *canbus.FilterSet
	recvOwn bool
	passive bool // the command never transmits: slcan adapters open listen-only
}

// slcanFlags are the --slcan-* flags of an invocation.
type slcanFlags struct{ bitrate, dataBitrate, baud int }

// slcanSettings are the flags of the running invocation (tests run several
// at once in one process).
var (
	slcanMu       sync.Mutex
	slcanSettings = slcanFlags{bitrate: 500000, baud: 115200}
)

func setSlcan(f slcanFlags) {
	slcanMu.Lock()
	slcanSettings = f
	slcanMu.Unlock()
}

func slcanConfig() slcanFlags {
	slcanMu.Lock()
	defer slcanMu.Unlock()
	return slcanSettings
}

// openSlcan opens "slcan:<device>" (a serial adapter) or
// "slcan:tcp://host:port" (an adapter behind a serial server).
func openSlcan(iface string, o openOpts) (canbus.Transport, error) {
	spec := strings.TrimPrefix(iface, cli.SlcanPrefix)
	set := slcanConfig()
	cfg := slcan.Config{Bitrate: set.bitrate, DataBitrate: set.dataBitrate, ListenOnly: o.passive, Name: iface}
	var c *slcan.Conn
	var err error
	if addr, ok := strings.CutPrefix(spec, "tcp://"); ok {
		var nc net.Conn
		if nc, err = net.DialTimeout("tcp", addr, 5*time.Second); err == nil {
			c, err = slcan.New(nc, cfg)
		}
	} else {
		c, err = slcan.Open(spec, set.baud, cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", canbus.ErrInterfaceDown, iface, err)
	}
	if o.filters != nil && (len(o.filters.Filters) > 0 || o.filters.Join) {
		return &softFiltered{c, *o.filters}, nil
	}
	return c, nil
}

// softFiltered applies filters in software: slcan adapters have none.
type softFiltered struct {
	*slcan.Conn
	fs canbus.FilterSet
}

func (s *softFiltered) Receive(ctx context.Context, f *canbus.Frame) error {
	for {
		if err := s.Conn.Receive(ctx, f); err != nil {
			return err
		}
		if s.fs.Match(f) {
			return nil
		}
	}
}

// The in-process virtual bus ("--interface virtual"), shared by the ports
// of one invocation.
var (
	virtualMu  sync.Mutex
	virtualBus *virtual.Bus
)

func sharedVirtual() *virtual.Bus {
	virtualMu.Lock()
	defer virtualMu.Unlock()
	if virtualBus == nil {
		virtualBus = virtual.New(virtual.Config{Name: cli.Virtual})
	}
	return virtualBus
}

// openTransport opens an interface; tests replace it.
var openTransport = func(iface string, o openOpts) (canbus.Transport, error) {
	if strings.HasPrefix(iface, cli.SlcanPrefix) {
		return openSlcan(iface, o)
	}
	if iface == cli.Virtual {
		vo := []virtual.PortOption{virtual.WithFD(), virtual.WithQueueSize(65536)}
		if o.filters != nil {
			vo = append(vo, virtual.WithFilters(*o.filters))
		}
		if o.recvOwn {
			vo = append(vo, virtual.WithRecvOwn())
		}
		return sharedVirtual().Open(vo...)
	}
	var so []socketcan.Option
	fd := o.fd
	if o.autoFD && !fd {
		if it, err := link.Get(iface); err == nil && it.FD() {
			fd = true
		}
	}
	if fd {
		so = append(so, socketcan.WithFD())
	}
	if o.filters != nil {
		so = append(so, socketcan.WithFilters(*o.filters))
	}
	if o.recvOwn {
		so = append(so, socketcan.WithRecvOwn())
	}
	return socketcan.Open(iface, so...)
}

// openBus opens an interface wrapped in a Bus that reads from its first
// subscription on: no frame between opening and subscribing is lost.
func openBus(iface string, o openOpts) (*canbus.Bus, error) {
	t, err := openTransport(iface, o)
	if err != nil {
		return nil, err
	}
	b, err := canbus.NewBus(t, canbus.WithInterfaceName(iface), canbus.WithStartOnSubscribe())
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	return b, nil
}

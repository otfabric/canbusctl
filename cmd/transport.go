// SPDX-License-Identifier: MIT

package cmd

import (
	"sync"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/go-canbus"
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

// openBus opens an interface wrapped in a Bus.
func openBus(iface string, o openOpts) (*canbus.Bus, error) {
	t, err := openTransport(iface, o)
	if err != nil {
		return nil, err
	}
	b, err := canbus.NewBus(t, canbus.WithInterfaceName(iface))
	if err != nil {
		_ = t.Close()
		return nil, err
	}
	return b, nil
}

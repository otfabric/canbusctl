// SPDX-License-Identifier: MIT

package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus/socketcan/link"
	"github.com/spf13/cobra"
)

type interfacesResult struct {
	Interfaces []link.Interface `json:"interfaces"`
}

func ifaceState(it *link.Interface) string {
	s := "DOWN"
	if it.Up {
		s = "UP"
	}
	if it.State != nil {
		s += "/" + it.State.String()
	}
	return s
}

func bitrate(it *link.Interface) string {
	switch {
	case it.BitTiming == nil || it.BitTiming.Bitrate == 0:
		return "-"
	case it.DataBitTiming != nil && it.DataBitTiming.Bitrate != 0:
		return fmt.Sprintf("%d/%d", it.BitTiming.Bitrate, it.DataBitTiming.Bitrate)
	}
	return strconv.FormatUint(uint64(it.BitTiming.Bitrate), 10)
}

func (r interfacesResult) Headers() []string {
	return []string{"NAME", "KIND", "STATE", "MTU", "FD", "BITRATE"}
}

func (r interfacesResult) Rows() [][]string {
	var rows [][]string
	for i := range r.Interfaces {
		it := &r.Interfaces[i]
		rows = append(rows, []string{it.Name, kindOf(it), ifaceState(it), strconv.Itoa(it.MTU), strconv.FormatBool(it.FD()), bitrate(it)})
	}
	return rows
}

func kindOf(it *link.Interface) string {
	if it.Kind == "" {
		return "?"
	}
	return it.Kind
}

func (r interfacesResult) Text() string {
	if len(r.Interfaces) == 0 {
		return "no CAN interfaces"
	}
	var b strings.Builder
	for i := range r.Interfaces {
		it := &r.Interfaces[i]
		fmt.Fprintf(&b, "%-10s %-6s %-16s mtu %-4d bitrate %s\n", it.Name, kindOf(it), ifaceState(it), it.MTU, bitrate(it))
	}
	return b.String()
}

func newInterfacesCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "interfaces",
		Short: "List the CAN interfaces of this host (Linux)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			list, err := link.List()
			if err != nil {
				return err
			}
			return a.emit("interfaces", interfacesResult{Interfaces: list})
		},
	}
}

type infoResult struct {
	link.Interface
}

func (r infoResult) Text() string {
	it := &r.Interface
	var b strings.Builder
	fmt.Fprintf(&b, "%s (index %d, kind %s)\n", it.Name, it.Index, kindOf(it))
	fmt.Fprintf(&b, "  state       %s (oper %s)\n", ifaceState(it), it.OperState)
	fmt.Fprintf(&b, "  mtu         %d (%s), txqueuelen %d\n", it.MTU, map[bool]string{true: "CAN FD", false: "classical CAN"}[it.FD()], it.TxQueueLen)
	if it.BitTiming != nil {
		fmt.Fprintf(&b, "  bitrate     %d, sample point %.1f%%\n", it.BitTiming.Bitrate, float64(it.BitTiming.SamplePoint)/10)
	}
	if it.DataBitTiming != nil {
		fmt.Fprintf(&b, "  dbitrate    %d, sample point %.1f%%\n", it.DataBitTiming.Bitrate, float64(it.DataBitTiming.SamplePoint)/10)
	}
	if it.ClockHz != 0 {
		fmt.Fprintf(&b, "  clock       %d Hz\n", it.ClockHz)
	}
	if it.CtrlModeSupported != 0 || it.CtrlMode != 0 {
		fmt.Fprintf(&b, "  ctrlmode    %s (supported %s)\n", it.CtrlMode, it.CtrlModeSupported)
	}
	if it.RestartMs != nil {
		fmt.Fprintf(&b, "  restart-ms  %d\n", *it.RestartMs)
	}
	if it.ErrorCounters != nil {
		fmt.Fprintf(&b, "  errors      tx %d rx %d\n", it.ErrorCounters.TX, it.ErrorCounters.RX)
	}
	s := it.Stats
	fmt.Fprintf(&b, "  rx          %d frames, %d bytes, %d errors, %d dropped\n", s.RxPackets, s.RxBytes, s.RxErrors, s.RxDropped)
	fmt.Fprintf(&b, "  tx          %d frames, %d bytes, %d errors, %d dropped\n", s.TxPackets, s.TxBytes, s.TxErrors, s.TxDropped)
	if d := it.DeviceStats; d != nil {
		fmt.Fprintf(&b, "  controller  bus-error %d, warning %d, passive %d, bus-off %d, arbitration-lost %d, restarts %d\n",
			d.BusError, d.ErrorWarning, d.ErrorPassive, d.BusOff, d.ArbitrationLost, d.Restarts)
	}
	return b.String()
}

func newInfoCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "info <interface>",
		Short: "Show an interface's configuration, controller state and counters (Linux)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			it, err := link.Get(args[0])
			if err != nil {
				return err
			}
			return a.emit("info", infoResult{it})
		},
	}
}

type actionResult struct {
	Interface string   `json:"interface"`
	Action    string   `json:"action"`
	Changes   []string `json:"changes,omitempty"`
}

func (r actionResult) Text() string {
	if len(r.Changes) == 0 {
		return fmt.Sprintf("%s: %s done", r.Interface, r.Action)
	}
	return fmt.Sprintf("%s: %s done: %s", r.Interface, r.Action, strings.Join(r.Changes, ", "))
}

func needYes(yes bool, what string) error {
	if yes {
		return nil
	}
	return errs.New(errs.KindSafety, errs.CodeNeedsYes, "%s changes a live interface and interrupts its traffic: repeat with --yes", what)
}

func newUpDownCmd(a *app, up bool) *cobra.Command {
	name := map[bool]string{true: "up", false: "down"}[up]
	var yes bool
	c := &cobra.Command{
		Use:   name + " <interface>",
		Short: "Bring an interface " + name + " (Linux, CAP_NET_ADMIN, needs --yes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := needYes(yes, name); err != nil {
				return err
			}
			f := link.SetDown
			if up {
				f = link.SetUp
			}
			if err := f(args[0]); err != nil {
				return err
			}
			return a.emit(name, actionResult{Interface: args[0], Action: name})
		},
	}
	c.Flags().BoolVar(&yes, "yes", false, "Confirm the administrative change")
	return c
}

func newConfigureCmd(a *app) *cobra.Command {
	var (
		yes                      bool
		bitrateV, dbitrate       uint32
		sample, dsample          float64
		fd, listenOnly, loopback bool
		oneShot, berr            bool
		restartMs                uint32
		mtu                      int
		restart                  bool
	)
	c := &cobra.Command{
		Use:   "configure <interface>",
		Short: "Set bit rates, CAN FD and controller modes (Linux, CAP_NET_ADMIN, needs --yes)",
		Long: `configure changes a CAN controller's configuration through rtnetlink, as
"ip link set <if> type can ..." does. The kernel accepts bit timing only while the
interface is down: run "canbusctl down" first and "canbusctl up" after. vcan and
vxcan interfaces have no bit timing (exit 8); --mtu 16|72 switches their CAN FD
capability. Nothing is changed without --yes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := link.Config{Bitrate: bitrateV, DataBitrate: dbitrate}
			var changes []string
			if bitrateV != 0 {
				changes = append(changes, fmt.Sprintf("bitrate %d", bitrateV))
			}
			if dbitrate != 0 {
				changes = append(changes, fmt.Sprintf("dbitrate %d", dbitrate))
			}
			for _, sp := range []struct {
				v   float64
				dst *uint32
				n   string
			}{{sample, &cfg.SamplePoint, "sample-point"}, {dsample, &cfg.DataSamplePoint, "dsample-point"}} {
				if sp.v != 0 {
					if sp.v <= 0 || sp.v >= 100 {
						return errs.Invalid("--%s %v: a percentage between 0 and 100", sp.n, sp.v)
					}
					*sp.dst = uint32(sp.v*10 + 0.5)
					changes = append(changes, fmt.Sprintf("%s %.1f%%", sp.n, sp.v))
				}
			}
			for _, m := range []struct {
				flag string
				on   bool
				mode link.CtrlMode
			}{{"fd", fd, link.ModeFD}, {"listen-only", listenOnly, link.ModeListenOnly}, {"loopback", loopback, link.ModeLoopback},
				{"one-shot", oneShot, link.ModeOneShot}, {"berr-reporting", berr, link.ModeBerrReporting}} {
				if !cmd.Flags().Changed(m.flag) {
					continue
				}
				if m.on {
					cfg.Set |= m.mode
				} else {
					cfg.Clear |= m.mode
				}
				changes = append(changes, fmt.Sprintf("%s %v", m.flag, m.on))
			}
			if cmd.Flags().Changed("restart-ms") {
				cfg.RestartMs = &restartMs
				changes = append(changes, fmt.Sprintf("restart-ms %d", restartMs))
			}
			if cmd.Flags().Changed("mtu") {
				if mtu != 16 && mtu != 72 {
					return errs.Invalid("--mtu %d: 16 (classical CAN) or 72 (CAN FD)", mtu)
				}
				changes = append(changes, fmt.Sprintf("mtu %d", mtu))
			}
			if restart {
				changes = append(changes, "restart")
			}
			if len(changes) == 0 {
				return errs.Invalid("nothing to configure: give --bitrate, --fd, --mtu, --restart, ...")
			}
			if err := needYes(yes, "configure"); err != nil {
				return err
			}
			a.warn("changing %s: %s", args[0], strings.Join(changes, ", "))
			if !cfg.Empty() {
				if err := link.Configure(args[0], cfg); err != nil {
					return err
				}
			}
			if cmd.Flags().Changed("mtu") {
				if err := link.SetMTU(args[0], mtu); err != nil {
					return err
				}
			}
			if restart {
				if err := link.Restart(args[0]); err != nil {
					return err
				}
			}
			return a.emit("configure", actionResult{Interface: args[0], Action: "configure", Changes: changes})
		},
	}
	f := c.Flags()
	f.BoolVar(&yes, "yes", false, "Confirm the change")
	f.Uint32Var(&bitrateV, "bitrate", 0, "Nominal bit rate in bit/s")
	f.Float64Var(&sample, "sample-point", 0, "Nominal sample point in percent (e.g. 87.5)")
	f.Uint32Var(&dbitrate, "dbitrate", 0, "CAN FD data bit rate in bit/s")
	f.Float64Var(&dsample, "dsample-point", 0, "CAN FD data sample point in percent")
	f.BoolVar(&fd, "fd", false, "Switch CAN FD mode on (--fd) or off (--fd=false)")
	f.BoolVar(&listenOnly, "listen-only", false, "Listen-only mode: the controller never transmits nor acknowledges")
	f.BoolVar(&loopback, "loopback", false, "Controller loopback mode")
	f.BoolVar(&oneShot, "one-shot", false, "One-shot mode: no automatic retransmission")
	f.BoolVar(&berr, "berr-reporting", false, "Report bus errors as error frames")
	f.Uint32Var(&restartMs, "restart-ms", 0, "Automatic restart delay after bus-off in ms (0: off)")
	f.IntVar(&mtu, "mtu", 0, "MTU: 16 (classical) or 72 (CAN FD)")
	f.BoolVar(&restart, "restart", false, "Restart a bus-off controller")
	return c
}

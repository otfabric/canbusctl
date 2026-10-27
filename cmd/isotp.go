// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/otfabric/canbusctl/internal/cli"
	"github.com/otfabric/canbusctl/internal/errs"
	"github.com/otfabric/go-canbus"
	"github.com/otfabric/go-canbus/isotp"
	"github.com/otfabric/go-canbus/socketcan"
	"github.com/spf13/cobra"
)

type isotpFlags struct {
	iface, txID, rxID, addressing, txExt, rxExt, padding string
	extendedIDs, fd, brs, kernel                         bool
	txDL                                                 int
	bs                                                   uint8
	stmin                                                time.Duration
	maxSize                                              int
}

func (o *isotpFlags) add(c *cobra.Command, needTx bool) {
	f := c.Flags()
	f.StringVarP(&o.iface, "interface", "i", "", "CAN interface, or \"virtual\" (required)")
	f.StringVar(&o.txID, "tx-id", "", "Identifier of the frames this side sends (hexadecimal)")
	f.StringVar(&o.rxID, "rx-id", "", "Identifier of the frames this side receives (hexadecimal)")
	f.BoolVar(&o.extendedIDs, "extended-ids", false, "29-bit identifiers")
	f.StringVar(&o.addressing, "addressing", "normal", "normal, extended or mixed")
	f.StringVar(&o.txExt, "tx-ext", "", "Target address / address extension of sent frames (extended, mixed)")
	f.StringVar(&o.rxExt, "rx-ext", "", "First byte expected in received frames (extended, mixed)")
	f.BoolVar(&o.fd, "fd", false, "CAN FD frames")
	f.IntVar(&o.txDL, "tx-dl", 0, "CAN FD data length of sent frames (8..64; default 64)")
	f.BoolVar(&o.brs, "brs", false, "CAN FD bit rate switching")
	f.Uint8Var(&o.bs, "bs", 0, "Block size this receiver grants (0: all)")
	f.DurationVar(&o.stmin, "stmin", 0, "STmin this receiver asks for (0..127ms, or 100µs..900µs)")
	f.StringVar(&o.padding, "padding", "", "Pad frames with this byte (hexadecimal, e.g. CC)")
	f.IntVar(&o.maxSize, "max-size", 4095, "Largest message accepted")
	f.BoolVar(&o.kernel, "kernel", false, "Use the Linux kernel's ISO-TP sockets instead of the userspace implementation")
	_ = c.MarkFlagRequired("interface")
	_ = needTx
}

func (o *isotpFlags) config() (isotp.Config, error) {
	var cfg isotp.Config
	var err error
	if o.txID == "" || o.rxID == "" {
		return cfg, errs.Invalid("--tx-id and --rx-id are required")
	}
	if cfg.TxID, err = cli.ParseID(o.txID, o.extendedIDs); err != nil {
		return cfg, err
	}
	if cfg.RxID, err = cli.ParseID(o.rxID, o.extendedIDs); err != nil {
		return cfg, err
	}
	cfg.ExtendedIDs = o.extendedIDs
	switch strings.ToLower(o.addressing) {
	case "normal":
	case "extended":
		cfg.Addressing = isotp.Extended
	case "mixed":
		cfg.Addressing = isotp.Mixed
	default:
		return cfg, errs.Invalid("--addressing %q: normal, extended or mixed", o.addressing)
	}
	if cfg.Addressing != isotp.Normal {
		if o.txExt == "" || o.rxExt == "" {
			return cfg, errs.Invalid("--addressing %s needs --tx-ext and --rx-ext", o.addressing)
		}
		if cfg.TxAddress, err = cli.ParseHexByte(o.txExt); err != nil {
			return cfg, err
		}
		if cfg.RxAddress, err = cli.ParseHexByte(o.rxExt); err != nil {
			return cfg, err
		}
	}
	cfg.FD, cfg.TxDL, cfg.BRS, cfg.BlockSize, cfg.STmin, cfg.MaxMessageSize = o.fd, o.txDL, o.brs, o.bs, o.stmin, o.maxSize
	if o.padding != "" {
		b, err := cli.ParseHexByte(o.padding)
		if err != nil {
			return cfg, err
		}
		cfg.Padding, cfg.PadByte = true, &b
	}
	return cfg, nil
}

// endpoint opens a userspace or kernel ISO-TP endpoint.
func (o *isotpFlags) endpoint() (isotp.Endpoint, func(), error) {
	cfg, err := o.config()
	if err != nil {
		return nil, nil, err
	}
	if o.kernel {
		if o.iface == cli.Virtual {
			return nil, nil, errs.Invalid("--kernel needs a SocketCAN interface, not the in-process virtual bus")
		}
		k, err := socketcan.DialISOTP(o.iface, cfg)
		if err != nil {
			return nil, nil, err
		}
		return k, func() { _ = k.Close() }, nil
	}
	bus, err := openBus(o.iface, openOpts{fd: o.fd})
	if err != nil {
		return nil, nil, err
	}
	c, err := isotp.Dial(bus, cfg)
	if err != nil {
		_ = bus.Close()
		return nil, nil, err
	}
	return c, func() { _ = c.Close(); _ = bus.Close() }, nil
}

func newISOTPCmd(a *app) *cobra.Command {
	c := &cobra.Command{
		Use:   "isotp",
		Short: "Send, receive and monitor ISO-TP (ISO 15765-2) messages",
		Long: `ISO-TP carries messages of up to 4095 bytes (more with CAN FD escape) over CAN.
The userspace implementation of go-canbus is used by default; --kernel uses the
Linux kernel's CAN_ISOTP sockets. UDS and other services on top of ISO-TP are not
interpreted.`,
	}
	c.AddCommand(newISOTPSendCmd(a), newISOTPReceiveCmd(a), newISOTPMonitorCmd(a))
	return c
}

type isotpSendResult struct {
	Interface string `json:"interface"`
	Messages  int    `json:"messages"`
	Bytes     int    `json:"bytes"`
}

func (r isotpSendResult) Text() string {
	return fmt.Sprintf("sent %d message(s), %d bytes, on %s", r.Messages, r.Bytes, r.Interface)
}

func newISOTPSendCmd(a *app) *cobra.Command {
	var (
		o       isotpFlags
		data    string
		size    int
		count   int
		yes     bool
		timeout time.Duration
	)
	c := &cobra.Command{
		Use:   "send",
		Short: "Send ISO-TP messages (needs --yes on a non-virtual interface)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var payload []byte
			switch {
			case data != "" && size > 0:
				return errs.Invalid("give --data or --size, not both")
			case data != "":
				p, err := parseHexData(data)
				if err != nil {
					return err
				}
				payload = p
			case size > 0:
				payload = make([]byte, size)
				for i := range payload {
					payload[i] = byte(i % 251)
				}
			default:
				return errs.Invalid("give --data <hex> or --size <n>")
			}
			if count < 1 || count > 10000 {
				return errs.Invalid("--count %d: 1..10000", count)
			}
			if err := cli.TransmitCheck(o.iface, yes, "isotp send"); err != nil {
				return err
			}
			ep, closer, err := o.endpoint()
			if err != nil {
				return err
			}
			defer closer()
			ctx, cancel := bounded(cmd.Context(), timeout)
			defer cancel()
			for i := 0; i < count; i++ {
				if err := ep.Send(ctx, payload); err != nil {
					return err
				}
			}
			return a.emit("isotp send", isotpSendResult{Interface: o.iface, Messages: count, Bytes: count * len(payload)})
		},
	}
	o.add(c, true)
	c.Flags().StringVar(&data, "data", "", "Payload in hexadecimal")
	c.Flags().IntVar(&size, "size", 0, "Send a deterministic payload of this size (byte i = i mod 251)")
	c.Flags().IntVarP(&count, "count", "n", 1, "Messages to send")
	c.Flags().BoolVar(&yes, "yes", false, "Confirm transmission on a non-virtual interface")
	c.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Overall time limit")
	return c
}

type isotpMessage struct {
	Len  int    `json:"len"`
	Data string `json:"data"`
	TxID string `json:"txId,omitempty"`
}

func newISOTPReceiveCmd(a *app) *cobra.Command {
	var (
		o       isotpFlags
		count   int
		timeout time.Duration
	)
	c := &cobra.Command{
		Use:   "receive",
		Short: "Receive ISO-TP messages (sends flow control frames)",
		Long: `receive takes part in transfers: it answers first frames with flow control
frames (--bs, --stmin). Use "isotp monitor" to observe without sending anything.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ep, closer, err := o.endpoint()
			if err != nil {
				return err
			}
			defer closer()
			ctx, cancel := bounded(cmd.Context(), timeout)
			defer cancel()
			st := a.stream()
			for n := 0; count <= 0 || n < count; n++ {
				msg, err := ep.Receive(ctx)
				if err != nil {
					if ended(ctx, err) {
						if count > 0 {
							return errs.New(errs.KindTimeout, errs.CodeTimeout, "received %d of %d messages", n, count)
						}
						return nil
					}
					return err
				}
				m := isotpMessage{Len: len(msg), Data: hex.EncodeToString(msg)}
				if err := st.Record("message", fmt.Sprintf("%d bytes: %s", m.Len, strings.ToUpper(m.Data)), m); err != nil {
					return err
				}
			}
			return nil
		},
	}
	o.add(c, false)
	c.Flags().IntVarP(&count, "count", "n", 1, "Messages to receive (0: until the timeout or Ctrl-C)")
	c.Flags().DurationVar(&timeout, "timeout", 0, "Stop after this time (0: no limit)")
	return c
}

// passive reassembles ISO-TP messages of one identifier without sending
// flow control (listen-only).
type passive struct {
	off  int
	buf  []byte
	want int
	sn   byte
	on   bool
}

// feed processes one frame; it returns a complete message, or a note.
func (p *passive) feed(data []byte) (msg []byte, note string) {
	if len(data) <= p.off {
		return nil, ""
	}
	d := data[p.off:]
	switch d[0] >> 4 {
	case 0:
		l, start := int(d[0]&0x0F), 1
		if l == 0 && len(d) > 2 {
			l, start = int(d[1]), 2
		}
		if l == 0 || start+l > len(d) {
			return nil, "malformed single frame"
		}
		p.on = false
		return append([]byte(nil), d[start:start+l]...), ""
	case 1:
		if len(d) < 2 {
			return nil, "malformed first frame"
		}
		l, start := int(d[0]&0x0F)<<8|int(d[1]), 2
		if l == 0 && len(d) >= 6 {
			l, start = int(uint32(d[2])<<24|uint32(d[3])<<16|uint32(d[4])<<8|uint32(d[5])), 6
		}
		if l > 1<<20 {
			p.on = false
			return nil, fmt.Sprintf("first frame announces %d bytes: ignored", l)
		}
		p.buf, p.want, p.sn, p.on = append([]byte(nil), d[start:]...), l, 1, true
		if len(p.buf) >= l {
			p.on = false
			return p.buf[:l], ""
		}
		return nil, ""
	case 2:
		if !p.on {
			return nil, ""
		}
		if d[0]&0x0F != p.sn {
			p.on = false
			return nil, fmt.Sprintf("sequence number %d, expected %d: message dropped", d[0]&0x0F, p.sn)
		}
		p.sn = (p.sn + 1) & 0x0F
		p.buf = append(p.buf, d[1:]...)
		if len(p.buf) >= p.want {
			p.on = false
			return p.buf[:p.want], ""
		}
	case 3:
		if len(d) >= 3 {
			return nil, fmt.Sprintf("flow control status %d bs %d stmin 0x%02X", d[0]&0x0F, d[1], d[2])
		}
	}
	return nil, ""
}

func newISOTPMonitorCmd(a *app) *cobra.Command {
	var (
		o        isotpFlags
		duration time.Duration
		fcNotes  bool
	)
	c := &cobra.Command{
		Use:   "monitor",
		Short: "Observe ISO-TP transfers between --tx-id and --rx-id without transmitting",
		Long: `monitor reassembles the messages in both directions of an ISO-TP connection
from the frames on the bus (listen-only: it never sends flow control). --flow-control
also prints the flow control frames.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.kernel {
				return errs.New(errs.KindUnsupported, errs.CodeUnsupported, "isotp monitor observes frames itself; --kernel does not apply")
			}
			cfg, err := o.config()
			if err != nil {
				return err
			}
			fmtOf := canbus.ExactStandard
			if cfg.ExtendedIDs {
				fmtOf = canbus.ExactExtended
			}
			fs := canbus.FilterSet{Filters: []canbus.Filter{fmtOf(cfg.TxID), fmtOf(cfg.RxID)}}
			bus, err := openBus(o.iface, openOpts{autoFD: true, filters: &fs})
			if err != nil {
				return err
			}
			defer func() { _ = bus.Close() }()
			sub, _ := bus.Subscribe(canbus.WithQueueSize(65536))
			ctx, cancel := bounded(cmd.Context(), duration)
			defer cancel()
			off := 0
			if cfg.Addressing != isotp.Normal {
				off = 1
			}
			streams := map[canbus.ID]*passive{cfg.TxID: {off: off}, cfg.RxID: {off: off}}
			st := a.stream()
			for {
				f, err := sub.Receive(ctx)
				if err != nil {
					if ended(ctx, err) {
						return nil
					}
					return err
				}
				p := streams[f.ID]
				if p == nil {
					continue
				}
				msg, note := p.feed(f.Payload())
				id := idHex(f.ID, f.IsExtended())
				switch {
				case msg != nil:
					m := isotpMessage{Len: len(msg), Data: hex.EncodeToString(msg), TxID: id}
					if err := st.Record("message", fmt.Sprintf("%s: %d bytes: %s", id, m.Len, strings.ToUpper(m.Data)), m); err != nil {
						return err
					}
				case note != "" && (fcNotes || !strings.HasPrefix(note, "flow control")):
					_ = st.Record("note", id+": "+note, map[string]string{"txId": id, "note": note})
				}
			}
		},
	}
	o.add(c, false)
	c.Flags().DurationVar(&duration, "duration", 0, "Stop after this time")
	c.Flags().BoolVar(&fcNotes, "flow-control", false, "Print flow control frames")
	return c
}

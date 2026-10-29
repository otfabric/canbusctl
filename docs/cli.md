# canbusctl command reference

Generated from `canbusctl <command> --help` by scripts/gen-cli-doc.sh; do not edit.
Exit codes: [exitcodes.md](exitcodes.md). JSON output: [json-output.md](json-output.md).

```text
canbusctl is a command-line toolkit for CAN, CAN FD, CAN XL, ISO-TP and DBC/KCD
databases, built on github.com/otfabric/go-canbus.

Passive by default: monitor, record, stats, xl monitor and the dbc commands
never transmit. Commands that transmit (send, send-fd, xl send, replay, isotp
send, simulate, inject)
need --yes on any interface that is not virtual (vcan, vxcan or the in-process
bus "virtual"), and are bounded by --rate, --max-frames and --duration.

SocketCAN interfaces exist on Linux only; on macOS and Windows the database,
trace and virtual-bus commands work and interface commands exit 8.

Serial-line CAN adapters (CANable, CANtact, USBtin, LAWICEL) work on every
OS as --interface slcan:<device> (slcan:/dev/ttyACM0, slcan:COM3) or
slcan:tcp://host:port behind a serial server, opened at --slcan-bitrate;
passive commands open them listen-only.

CAN XL frames (xl send, xl monitor) need a SocketCAN interface with the CAN
XL MTU (ip link set vcanxl0 mtu 2060; Linux 6.2 or later).

Results go to stdout as text (default), a table or JSON (--format); progress
and diagnostics go to stderr. Exit codes: docs/exitcodes.md.

Usage:
  canbusctl [command]

Available Commands:
  bridge      Forward frames between two interfaces through seeded faults (needs --yes on a non-virtual interface)
  completion  Generate a shell completion script
  configure   Set bit rates, CAN FD and controller modes (Linux, CAP_NET_ADMIN, needs --yes)
  dbc         Inspect, validate, decode, encode, generate and convert CAN databases (DBC, KCD, ARXML)
  down        Bring an interface down (Linux, CAP_NET_ADMIN, needs --yes)
  help        Help about any command
  info        Show an interface's configuration, controller state and counters (Linux)
  inject      Pass traffic through controlled faults onto a virtual test bus
  interfaces  List the CAN interfaces of this host (Linux)
  isotp       Send, receive and monitor ISO-TP (ISO 15765-2) messages
  monitor     Print the frames of an interface as they arrive (passive)
  record      Record frames to a candump log, JSON Lines, pcap, pcapng, ASC, BLF or TRC file (passive)
  replay      Transmit a recorded trace (needs --yes on a non-virtual interface)
  send        Transmit classical CAN frames (needs --yes on a non-virtual interface)
  send-fd     Transmit CAN FD frames (needs --yes on a non-virtual interface)
  simulate    Generate deterministic traffic of a database's messages at their cycle times
  stats       Show interface counters and, with --duration, count live traffic per identifier (passive)
  trace       Convert and inspect trace files (candump, JSON Lines, pcap/pcapng, ASC, BLF, TRC)
  up          Bring an interface up (Linux, CAP_NET_ADMIN, needs --yes)
  version     Print the version of canbusctl and go-canbus
  xl          CAN XL frames on SocketCAN (Linux ≥ 6.2, interface mtu 2060) or the virtual bus

Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
  -h, --help                     help for canbusctl
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none

Use "canbusctl [command] --help" for more information about a command.
```

## canbusctl interfaces

```text
List the CAN interfaces of this host (Linux)

Usage:
  canbusctl interfaces [flags]

Flags:
  -h, --help   help for interfaces

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl info

```text
Show an interface's configuration, controller state and counters (Linux)

Usage:
  canbusctl info <interface> [flags]

Flags:
  -h, --help   help for info

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl configure

```text
configure changes a CAN controller's configuration through rtnetlink, as
"ip link set <if> type can ..." does. The kernel accepts bit timing only while the
interface is down: run "canbusctl down" first and "canbusctl up" after. vcan and
vxcan interfaces have no bit timing (exit 8); --mtu 16|72 switches their CAN FD
capability. Nothing is changed without --yes.

Usage:
  canbusctl configure <interface> [flags]

Flags:
      --berr-reporting        Report bus errors as error frames
      --bitrate uint32        Nominal bit rate in bit/s
      --dbitrate uint32       CAN FD data bit rate in bit/s
      --dsample-point float   CAN FD data sample point in percent
      --fd                    Switch CAN FD mode on (--fd) or off (--fd=false)
  -h, --help                  help for configure
      --listen-only           Listen-only mode: the controller never transmits nor acknowledges
      --loopback              Controller loopback mode
      --mtu int               MTU: 16 (classical) or 72 (CAN FD)
      --one-shot              One-shot mode: no automatic retransmission
      --restart               Restart a bus-off controller
      --restart-ms uint32     Automatic restart delay after bus-off in ms (0: off)
      --sample-point float    Nominal sample point in percent (e.g. 87.5)
      --yes                   Confirm the change

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl up

```text
Bring an interface up (Linux, CAP_NET_ADMIN, needs --yes)

Usage:
  canbusctl up <interface> [flags]

Flags:
  -h, --help   help for up
      --yes    Confirm the administrative change

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl down

```text
Bring an interface down (Linux, CAP_NET_ADMIN, needs --yes)

Usage:
  canbusctl down <interface> [flags]

Flags:
  -h, --help   help for down
      --yes    Confirm the administrative change

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl monitor

```text
monitor receives frames and prints them, one line or JSON record each, until
--count frames, --duration or Ctrl-C. It never transmits. --dbc decodes frames of
known messages with a DBC, KCD or ARXML database. --output also writes the
frames to a trace file (format by extension, as record). Filters use candump notation:
<id>:<mask> or <id>~<mask> (inverted), hexadecimal, with ",std", ",ext", ",rtr"
or ",data"; several filters are ORed, or ANDed with --join.

Usage:
  canbusctl monitor [flags]

Flags:
  -n, --count int            Stop after this many frames (0: no limit)
      --dbc string           Decode with this DBC, KCD or ARXML database
      --duration duration    Stop after this time (0: until Ctrl-C)
      --errors               Also receive error frames
      --filter stringArray   Filter <id>:<mask> (repeatable)
  -h, --help                 help for monitor
  -i, --interface string     CAN interface, or "virtual" (required)
      --join                 A frame must pass all filters (CAN_RAW_JOIN_FILTERS)
      --no-fd                Do not open the socket for CAN FD even when the interface carries it
  -o, --output string        Also write the frames to this trace file

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl send

```text
Frames are given in cansend notation: 123#DEADBEEF, 1234ABCD#11.22, 123#R,
123#R4, and for CAN FD 123##1<data> (the digit after ## is the flags nibble:
1 = BRS, 2 = ESI). Without arguments, --id and --data build one frame.
--repeat sends the frames that many times, --gap waits between frames.
On any interface that is not vcan, vxcan or "virtual" nothing is sent
without --yes.

Usage:
  canbusctl send [frame...] [flags]

Flags:
      --data string         Payload in hexadecimal
      --dlc int             Requested length of a remote frame
      --duration duration   Stop transmitting after this time (0: no limit)
      --extended            29-bit identifier
      --gap duration        Pause between frames
  -h, --help                help for send
      --id string           Identifier in hexadecimal (with --data)
  -i, --interface string    CAN interface, or "virtual" (required)
      --max-frames int      Transmit at most this many frames (default 10000)
      --rate float          Transmit at most this many frames per second (0: no limit)
      --repeat int          Send the frames this many times (default 1)
      --rtr                 Remote transmission request
      --yes                 Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl send-fd

```text
Frames are given in cansend notation: 123#DEADBEEF, 1234ABCD#11.22, 123#R,
123#R4, and for CAN FD 123##1<data> (the digit after ## is the flags nibble:
1 = BRS, 2 = ESI). Without arguments, --id and --data build one frame.
--repeat sends the frames that many times, --gap waits between frames.
On any interface that is not vcan, vxcan or "virtual" nothing is sent
without --yes.

Usage:
  canbusctl send-fd [frame...] [flags]

Flags:
      --brs                 Bit rate switching
      --data string         Payload in hexadecimal
      --duration duration   Stop transmitting after this time (0: no limit)
      --esi                 Set the error state indicator
      --extended            29-bit identifier
      --gap duration        Pause between frames
  -h, --help                help for send-fd
      --id string           Identifier in hexadecimal (with --data)
  -i, --interface string    CAN interface, or "virtual" (required)
      --max-frames int      Transmit at most this many frames (default 10000)
      --rate float          Transmit at most this many frames per second (0: no limit)
      --repeat int          Send the frames this many times (default 1)
      --yes                 Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl record

```text
record writes received frames to --output, in the format of its extension
(.log/.txt candump, .jsonl lossless JSON Lines, .pcap and .pcapng Wireshark,
.asc and .blf Vector, .trc PEAK; ASC/BLF/TRC times are local wall-clock
times: set TZ) or --file-format,
until --count, --duration or Ctrl-C. candump logs keep microseconds; JSON Lines
keeps every field, including nanosecond timestamps; pcapng keeps interface
names. --xl also records CAN XL frames (a SocketCAN interface with the CAN XL
MTU or the virtual bus; candump, JSON Lines, pcap and pcapng only).

Usage:
  canbusctl record [flags]

Flags:
  -n, --count int            Stop after this many frames
      --duration duration    Stop after this time
      --errors               Also record error frames
      --file-format string   candump, jsonl, pcap, pcapng, asc, blf or trc (default: from the extension)
      --filter stringArray   Filter <id>:<mask> (repeatable; selects CAN XL frames by priority)
  -h, --help                 help for record
  -i, --interface string     CAN interface, or "virtual" (required)
      --no-fd                Do not open the socket for CAN FD
  -o, --output string        Output file (required)
      --xl                   Also record CAN XL frames (SocketCAN with the CAN XL MTU, or virtual)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl replay

```text
replay sends the frames of a candump log, JSON Lines, pcap/pcapng, ASC or BLF file ("-":
stdin) with their recorded timing (--speed 1), scaled (--speed 2 is twice as fast)
or as fast as allowed (--max-speed). Error frames are never sent. --max-frames,
--duration and --rate bound the transmission. The trace is streamed, not loaded.
--xl opens the interface for CAN XL and sends the trace's CAN XL frames too;
without it they are counted, not sent.

Usage:
  canbusctl replay <trace> [flags]

Flags:
      --duration duration    Stop transmitting after this time (0: no limit)
      --filter stringArray   Replay only frames passing <id>:<mask> (repeatable)
  -h, --help                 help for replay
  -i, --interface string     CAN interface, or "virtual" (required)
      --max-frames int       Transmit at most this many frames (default 10000)
      --max-speed            Ignore the recorded timing
      --print                Print every frame sent
      --rate float           Transmit at most this many frames per second (0: no limit)
      --speed float          Timing factor: 1 original, 2 twice as fast (default 1)
      --xl                   Send CAN XL frames too (SocketCAN with the CAN XL MTU, or virtual)
      --yes                  Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl stats

```text
Show interface counters and, with --duration, count live traffic per identifier (passive)

Usage:
  canbusctl stats [flags]

Flags:
      --duration duration   Count live traffic for this long
  -h, --help                help for stats
  -i, --interface string    CAN interface, or "virtual" (required)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc

```text
The dbc commands take DBC files, KCD (Kayak) files and AUTOSAR 4.x ARXML
system descriptions (their CAN part), recognised by the .kcd and .arxml
extensions or by content. DBC is a database format describing messages and
signals, not a protocol. None of these commands touches a CAN interface.

Usage:
  canbusctl dbc [command]

Available Commands:
  convert     Convert a database: DBC, KCD or ARXML to DBC, KCD or the JSON model
  decode      Decode frames given as arguments, in a trace file (--trace) or on stdin
  encode      Encode a message from signal values into a frame (cansend notation)
  generate    Generate typed Go code (or a JSON descriptor) from a database
  inspect     List the messages of a database, or the signals of one (--message)
  validate    Report every diagnostic of a database; exit 1 on errors (or warnings with --strict)

Flags:
  -h, --help   help for dbc

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none

Use "canbusctl dbc [command] --help" for more information about a command.
```

## canbusctl dbc inspect

```text
List the messages of a database, or the signals of one (--message)

Usage:
  canbusctl dbc inspect <database> [flags]

Flags:
  -h, --help             help for inspect
  -m, --message string   Show the signals of this message (name or hexadecimal identifier)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc validate

```text
Report every diagnostic of a database; exit 1 on errors (or warnings with --strict)

Usage:
  canbusctl dbc validate <database> [flags]

Flags:
  -h, --help     help for validate
      --strict   Treat warnings as errors

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc decode

```text
decode prints the signals of each frame of a known message. Frames come as
cansend notation arguments, from a trace file (--trace: candump, JSON Lines,
pcap/pcapng, ASC or BLF; "-" for stdin), or, without either, as lines of
cansend notation or candump log on stdin. A frame that does not decode (too
short, bad multiplexer) makes the exit code 1; frames of unknown messages do
too with --strict.

Usage:
  canbusctl dbc decode <database> [frame...] [flags]

Flags:
  -h, --help           help for decode
      --strict         Exit 1 also for frames of unknown messages
      --trace string   Read frames from this trace file ("-": stdin)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc encode

```text
Values are physical (scaled) numbers, value-table names (Gear=Second), or raw
integers with --raw. Every signal the payload carries must be given (multiplexer
values select which), unless --fill-missing uses initial values. Physical values
outside a signal's [min, max] are rejected unless --lax.

Usage:
  canbusctl dbc encode <database> <message> [signal=value...] [flags]

Flags:
      --fill-missing   Encode missing signals with their initial value (or 0)
  -h, --help           help for encode
      --lax            Do not check physical values against the signal range
      --raw            Values are raw integers

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc generate

```text
generate writes gofmt-formatted Go: one struct per message with typed raw fields,
identifier constants, enums, physical accessors and Encode/Decode/Frame methods.
The output is deterministic and names the source file and its SHA-256. It imports
github.com/otfabric/go-canbus. --json writes the format-independent JSON
descriptor instead.

Usage:
  canbusctl dbc generate <database> [flags]

Flags:
  -h, --help             help for generate
      --json             Write the JSON descriptor instead of Go
  -o, --output string    Output file (default: stdout)
  -p, --package string   Go package name (default: from the file name)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl dbc convert

```text
Reads DBC, KCD or AUTOSAR 4.x ARXML (the CAN subset) and writes DBC, KCD
or the JSON model (--to dbc|kcd|json). Writing ARXML is not supported.
KCD cannot carry extended multiplexing (an error), attributes, value tables
and initial values (dropped, reported as warnings on stderr).

Usage:
  canbusctl dbc convert <database> [flags]

Flags:
  -h, --help            help for convert
  -o, --output string   Output file (default: stdout)
      --to string       Target format: dbc, kcd or json (required)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl isotp

```text
ISO-TP carries messages of up to 4095 bytes (more with CAN FD escape) over CAN.
The userspace implementation of go-canbus is used by default; --kernel uses the
Linux kernel's CAN_ISOTP sockets. UDS and other services on top of ISO-TP are not
interpreted.

Against the kernel's ISO-TP on vcan, a userspace receiver can answer a block
faster than the kernel sender sees the echo of its own last frame; the kernel
then drops the flow control and the transfer times out. Give the userspace
receiver --fc-delay 200us (N_Br) in that setup; real buses are slow enough.

Usage:
  canbusctl isotp [command]

Available Commands:
  monitor     Observe ISO-TP transfers between --tx-id and --rx-id without transmitting
  receive     Receive ISO-TP messages (sends flow control frames)
  send        Send ISO-TP messages (needs --yes on a non-virtual interface)

Flags:
  -h, --help   help for isotp

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none

Use "canbusctl isotp [command] --help" for more information about a command.
```

## canbusctl isotp send

```text
Send ISO-TP messages (needs --yes on a non-virtual interface)

Usage:
  canbusctl isotp send [flags]

Flags:
      --addressing string   normal, extended or mixed (default "normal")
      --brs                 CAN FD bit rate switching
      --bs uint8            Block size this receiver grants (0: all)
  -n, --count int           Messages to send (default 1)
      --data string         Payload in hexadecimal
      --extended-ids        29-bit identifiers
      --fc-delay duration   Wait this long before each flow control this receiver sends (N_Br, 0..1s; e.g. 200µs against the kernel's ISO-TP on vcan)
      --fd                  CAN FD frames
  -h, --help                help for send
  -i, --interface string    CAN interface, or "virtual" (required)
      --kernel              Use the Linux kernel's ISO-TP sockets instead of the userspace implementation
      --max-size int        Largest message accepted (default 4095)
      --padding string      Pad frames with this byte (hexadecimal, e.g. CC)
      --rx-ext string       First byte expected in received frames (extended, mixed)
      --rx-id string        Identifier of the frames this side receives (hexadecimal)
      --size int            Send a deterministic payload of this size (byte i = i mod 251)
      --stmin duration      STmin this receiver asks for (0..127ms, or 100µs..900µs)
      --timeout duration    Overall time limit (default 30s)
      --tx-dl int           CAN FD data length of sent frames (8..64; default 64)
      --tx-ext string       Target address / address extension of sent frames (extended, mixed)
      --tx-id string        Identifier of the frames this side sends (hexadecimal)
      --yes                 Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl isotp receive

```text
receive takes part in transfers: it answers first frames with flow control
frames (--bs, --stmin). Use "isotp monitor" to observe without sending anything.

Usage:
  canbusctl isotp receive [flags]

Flags:
      --addressing string   normal, extended or mixed (default "normal")
      --brs                 CAN FD bit rate switching
      --bs uint8            Block size this receiver grants (0: all)
  -n, --count int           Messages to receive (0: until the timeout or Ctrl-C) (default 1)
      --extended-ids        29-bit identifiers
      --fc-delay duration   Wait this long before each flow control this receiver sends (N_Br, 0..1s; e.g. 200µs against the kernel's ISO-TP on vcan)
      --fd                  CAN FD frames
  -h, --help                help for receive
  -i, --interface string    CAN interface, or "virtual" (required)
      --kernel              Use the Linux kernel's ISO-TP sockets instead of the userspace implementation
      --max-size int        Largest message accepted (default 4095)
      --padding string      Pad frames with this byte (hexadecimal, e.g. CC)
      --rx-ext string       First byte expected in received frames (extended, mixed)
      --rx-id string        Identifier of the frames this side receives (hexadecimal)
      --stmin duration      STmin this receiver asks for (0..127ms, or 100µs..900µs)
      --timeout duration    Stop after this time (0: no limit)
      --tx-dl int           CAN FD data length of sent frames (8..64; default 64)
      --tx-ext string       Target address / address extension of sent frames (extended, mixed)
      --tx-id string        Identifier of the frames this side sends (hexadecimal)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl isotp monitor

```text
monitor reassembles the messages in both directions of an ISO-TP connection
from the frames on the bus, with two listen-only ISO-TP connections of go-canbus:
it never transmits (no flow control). Reception errors (wrong sequence number,
N_Cr timeout) are printed as notes.

Usage:
  canbusctl isotp monitor [flags]

Flags:
      --addressing string   normal, extended or mixed (default "normal")
      --brs                 CAN FD bit rate switching
      --bs uint8            Block size this receiver grants (0: all)
      --duration duration   Stop after this time
      --extended-ids        29-bit identifiers
      --fc-delay duration   Wait this long before each flow control this receiver sends (N_Br, 0..1s; e.g. 200µs against the kernel's ISO-TP on vcan)
      --fd                  CAN FD frames
  -h, --help                help for monitor
  -i, --interface string    CAN interface, or "virtual" (required)
      --kernel              Use the Linux kernel's ISO-TP sockets instead of the userspace implementation
      --max-size int        Largest message accepted (default 4095)
      --padding string      Pad frames with this byte (hexadecimal, e.g. CC)
      --rx-ext string       First byte expected in received frames (extended, mixed)
      --rx-id string        Identifier of the frames this side receives (hexadecimal)
      --stmin duration      STmin this receiver asks for (0..127ms, or 100µs..900µs)
      --tx-dl int           CAN FD data length of sent frames (8..64; default 64)
      --tx-ext string       Target address / address extension of sent frames (extended, mixed)
      --tx-id string        Identifier of the frames this side sends (hexadecimal)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl trace

```text
The trace commands work on files only and never touch an interface.

Readable: candump logs (.log/.txt), JSON Lines (.jsonl), pcap and pcapng,
Vector ASC (.asc) and BLF (.blf), PEAK TRC (.trc, versions 1.0 to 2.1;
written as 2.1), detected from the content. Writable: all of them, chosen
by the output's extension or --file-format. CAN XL frames are kept by
candump logs, JSON Lines, pcap and pcapng.

ASC, BLF and TRC hold local wall-clock times without a time zone: they are
read and written in the local zone; set TZ (e.g. TZ=UTC) to match a file
from another zone.

Usage:
  canbusctl trace [command]

Available Commands:
  convert     Convert a trace to another format, optionally filtered
  info        Summarize a trace: frames, duration, kinds, per-identifier counts and rates

Flags:
  -h, --help   help for trace

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none

Use "canbusctl trace [command] --help" for more information about a command.
```

## canbusctl trace convert

```text
convert reads <input> ("-": stdin) in any readable format and writes
<output> in the format of its extension (or --file-format). --filter keeps
only matching frames (candump notation, repeatable, ORed or ANDed with
--join); --no-errors drops error frames; --max-frames stops early. What the
target format cannot keep (error class in ASC/BLF, interface names, echo
marks, sub-microsecond times, CAN XL frames in ASC/BLF/TRC) is reported,
never silently dropped. --filter selects CAN XL frames by priority, as the
kernel's filters do.

Usage:
  canbusctl trace convert <input> <output> [flags]

Flags:
      --file-format string   candump, jsonl, pcap, pcapng, asc, blf or trc (default: from the extension)
      --filter stringArray   Keep frames matching <id>:<mask> (repeatable)
  -h, --help                 help for convert
      --join                 A frame must pass all filters
      --max-frames int       Stop after writing this many frames (0: all)
      --no-errors            Drop error frames

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl trace info

```text
Summarize a trace: frames, duration, kinds, per-identifier counts and rates

Usage:
  canbusctl trace info <file> [flags]

Flags:
  -h, --help   help for info

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl simulate

```text
simulate sends the messages of a DBC, KCD or ARXML database at their cycle times
(GenMsgCycleTime / interval; --default-cycle for the others) with signal values
from --pattern: counter (the step number), random (seeded by --seed) or initial.
The same seed and database give the same frames. With --interface virtual the
traffic stays in this process and is printed; on a vcan or vxcan interface it
is sent; on any other interface it needs --yes. --duration (default 10s),
--max-frames and --rate bound it.

Usage:
  canbusctl simulate [flags]

Flags:
      --dbc string               DBC, KCD or ARXML database (required)
      --default-cycle duration   Period of messages without a cycle time (0: skip them)
      --duration duration        Stop transmitting after this time (0: no limit) (default 10s)
  -h, --help                     help for simulate
  -i, --interface string         CAN interface, or "virtual" (in this process) (default "virtual")
      --max-frames int           Transmit at most this many frames (default 10000)
      --message strings          Simulate only these messages (repeatable)
      --pattern string           Signal values: counter, random or initial (default "counter")
      --print                    Print every frame (always on for the virtual bus)
      --rate float               Transmit at most this many frames per second (0: no limit)
      --seed int                 Seed of the random pattern (default 1)
      --yes                      Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl inject

```text
inject takes frames (--frame in cansend notation, or a --trace file) through
a seeded fault stage (the chaos bridge of go-canbus, as in "bridge") —
--drop, --duplicate, --delay-probability with --delay, --reorder, --corrupt
(one payload bit flipped) — and delivers what survives:

  --interface virtual  the frames are printed as a receiver on the in-process
                       virtual bus sees them; --error-frame (bus-off, passive,
                       warning, no-ack, stuff, overflow, restarted) and
                       --malformed N (frames violating the length rules)
                       are injected there too.
  vcan / vxcan         the surviving frames are transmitted, faults applied in
                       userspace before sending. Error frames and malformed
                       frames cannot be sent through SocketCAN (exit 8).
  other interfaces     refused without --yes.

All faults are emulated in userspace. Arbitration, bit errors, CRC failures,
error confinement and electrical faults need real hardware and are not
produced. The same --seed gives the same faults.

Usage:
  canbusctl inject [flags]

Flags:
      --corrupt float             Probability one payload bit is flipped
      --delay duration            Hold time of delayed frames (default 10ms)
      --delay-probability float   Probability a frame is held for --delay (later frames wait behind it)
      --drop float                Probability a frame is lost
      --duplicate float           Probability a frame is forwarded twice
      --duration duration         Stop transmitting after this time (0: no limit)
      --error-frame string        Inject an error frame (virtual bus only)
      --frame stringArray         A frame in cansend notation (repeatable)
  -h, --help                      help for inject
  -i, --interface string          "virtual", a vcan/vxcan interface, or another with --yes (default "virtual")
      --malformed int             Inject this many malformed frames (virtual bus only)
      --max-frames int            Transmit at most this many frames (default 10000)
      --rate float                Transmit at most this many frames per second (0: no limit)
      --reorder float             Probability a frame is sent after the next one
      --seed int                  Seed of the fault generator (default 1)
      --trace string              Frames from a trace file
      --yes                       Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl bridge

```text
bridge forwards every frame received on -a to -b and, unless --one-way, from
-b to -a, through a seeded fault stage: --drop, --duplicate,
--delay-probability with --delay, --reorder, --corrupt. --filter limits the
faults to matching frames (others pass untouched). Peers connected to only
one side see each other through the bridge: put it between vcan0 and vcan1
to test a stack, or a device on a bench bus, under faults.

Interfaces: vcan/vxcan, "virtual" (the in-process bus), or others with --yes
(the bridge transmits). Error frames and a socket's own transmissions are not
forwarded. Runs for --duration (required) or until Ctrl-C, then prints the
counters per direction.

The faults are userspace emulation of what a receiver sees. Corrupted frames
are delivered, where a real bus would reject them by CRC; a single bus never
reorders a sender's frames. The same --seed gives the same decisions for the
same traffic.

Usage:
  canbusctl bridge [flags]

Flags:
      --corrupt float             Probability one payload bit is flipped
      --delay duration            Hold time of delayed frames (default 10ms)
      --delay-probability float   Probability a frame is held for --delay (later frames wait behind it)
      --drop float                Probability a frame is lost
      --duplicate float           Probability a frame is forwarded twice
      --duration duration         Run for this long (required)
      --filter stringArray        Apply faults only to frames matching <id>:<mask> (repeatable)
  -h, --help                      help for bridge
      --one-way                   Forward from -a to -b only
      --reorder float             Probability a frame is sent after the next one
      --seed int                  Seed of the fault generator (default 1)
  -a, --side-a string             First interface (required)
  -b, --side-b string             Second interface (required)
      --yes                       Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl xl

```text
CAN XL frames (ISO 11898-1:2024: 11-bit priority, VCID, SEC and RRS flags,
SDU type, acceptance field, 1..2048 data bytes) on a SocketCAN interface with
the CAN XL MTU (ip link set vcanxl0 mtu 2060; Linux 6.2 or later, VCIDs 6.9)
or the in-process virtual bus. record --xl and replay --xl carry CAN XL
frames too; trace convert keeps them where the target format can.
Frames use can-utils notation: <vcid><prio>#<flags>:<sdt>:<af>#<data>, for
example 00242#80:03:00C0FFEE#11223344, or <prio>#... with VCID 0.

Usage:
  canbusctl xl [command]

Available Commands:
  monitor     Print CAN, CAN FD and CAN XL frames as they arrive (passive)
  send        Transmit CAN XL frames (needs --yes on a non-virtual interface)

Flags:
  -h, --help   help for xl

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none

Use "canbusctl xl [command] --help" for more information about a command.
```

## canbusctl xl send

```text
Frames in can-utils notation, as cansend takes them:
<vcid><prio>#<flags>:<sdt>:<af>#<data> (data bytes may be separated by dots).
--repeat sends the frames that many times, --gap waits between frames. On any
interface that is not vcan or vxcan nothing is sent without --yes.

Usage:
  canbusctl xl send frame... [flags]

Flags:
      --duration duration   Stop transmitting after this time (0: no limit)
      --gap duration        Pause between frames
  -h, --help                help for send
  -i, --interface string    SocketCAN interface with the CAN XL MTU, or "virtual" (required)
      --max-frames int      Transmit at most this many frames (default 10000)
      --rate float          Transmit at most this many frames per second (0: no limit)
      --repeat int          Send the frames this many times (default 1)
      --yes                 Confirm transmission on a non-virtual interface

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl xl monitor

```text
xl monitor receives every frame of a CAN XL interface, CAN XL and CAN or
CAN FD alike, and prints them until --count frames, --duration or Ctrl-C. It
never transmits. --output also writes them to a candump log (.log), JSON
Lines (.jsonl), pcap or pcapng (Wireshark) trace, the formats that carry CAN
XL.

Usage:
  canbusctl xl monitor [flags]

Flags:
  -n, --count int           Stop after this many frames (0: no limit)
      --duration duration   Stop after this time (0: until Ctrl-C)
  -h, --help                help for monitor
  -i, --interface string    SocketCAN interface with the CAN XL MTU, or "virtual" (required)
  -o, --output string       Also write the frames to this trace file (.log, .jsonl, .pcap or .pcapng)

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl version

```text
Print the version of canbusctl and go-canbus

Usage:
  canbusctl version [flags]

Flags:
  -h, --help   help for version

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

## canbusctl completion

```text
Generate a shell completion script

Usage:
  canbusctl completion bash|zsh|fish|powershell [flags]

Flags:
  -h, --help   help for completion

Global Flags:
      --format string            Output format: text, json or table (env CANBUSCTL_FORMAT) (default "text")
      --slcan-baud int           Serial baud rate of an slcan adapter (USB adapters ignore it) (default 115200)
      --slcan-bitrate int        Bit rate an slcan:<device> adapter is opened with (0: keep the adapter's) (default 500000)
      --slcan-data-bitrate int   CAN FD data bit rate of an slcan adapter: 2000000 or 5000000 (CANable 2.0); 0 sends none
```

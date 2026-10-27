# Limitations

- **Hardware:** only Linux SocketCAN. No drivers for PEAK, Kvaser, Vector,
  slcan or other adapters, so no physical CAN on macOS or Windows (exit 8).
- **Fault injection is userspace emulation.** `inject` drops, duplicates,
  delays and corrupts frames before they are delivered; it cannot produce bit
  errors, CRC failures, arbitration loss, error confinement, bus-off or
  electrical faults, which need hardware. Error frames and malformed frames
  exist only on the in-process virtual bus: SocketCAN does not send them.
- **The virtual bus models delivery, not CAN:** no arbitration, timing or
  acknowledgement.
- **ISO-TP:** `isotp monitor` follows one connection (two identifiers); the
  kernel ISO-TP sockets use the kernel's fixed 1 s timeouts and cannot send
  WAIT flow controls. UDS and other services are not decoded.
- **Databases:** DBC and KCD in; DBC and JSON out. No ARXML, no KCD writer.
  Environment variables and other DBC constructs go-canbus ignores are
  reported as warnings (see go-canbus LIMITATIONS.md).
- **Traces:** candump logs keep microseconds; pcap does not keep interface
  names or the own-message mark; only JSON Lines is lossless. ASC and BLF are
  not read.
- **CAN XL** frames are not supported.
- `simulate` generates values from patterns (counter, random, initial); it does
  not model ECU behaviour or react to traffic.
- No device discovery or fingerprinting: a CAN identifier does not identify a
  device.

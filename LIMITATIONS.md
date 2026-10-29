# Limitations

Gaps and follow-ups, grouped and ordered: [ROADMAP.md](ROADMAP.md).

- **Hardware:** Linux SocketCAN, and serial-line CAN (slcan) adapters on
  every OS (`-i slcan:<device>`). No drivers for PEAK, Kvaser, Vector or
  other adapters. slcan delivers error frames only from adapters that send
  status records ("e", "s"); it has no error state indicator (ESI), echo or
  hardware filters (canbusctl filters in software), and a sent frame is not
  confirmed on the bus. Kernel ISO-TP (`--kernel`) needs
  a SocketCAN interface.
- **Fault injection is userspace emulation.** `inject` and `bridge` drop,
  duplicate, delay, reorder and corrupt frames before they are delivered (a
  real bus would reject a corrupted frame by CRC, and never reorders one
  sender's frames); it cannot produce bit
  errors, CRC failures, arbitration loss, error confinement, bus-off or
  electrical faults, which need hardware. Error frames and malformed frames
  exist only on the in-process virtual bus: SocketCAN does not send them.
- **The virtual bus models delivery, not CAN:** no arbitration, timing or
  acknowledgement.
- **ISO-TP:** `isotp monitor` follows one connection (two identifiers); the
  kernel ISO-TP sockets use the kernel's fixed 1 s timeouts and cannot send
  WAIT flow controls, and `--fc-delay` does not apply to them. On vcan the
  kernel's ISO-TP sender drops a flow control that arrives before the echo of
  its own last block frame: a fast userspace receiver (`isotp receive`
  against `isotp send --kernel`) needs `--fc-delay 200us` there; real buses
  are slow enough. UDS and other services are not decoded.
- **bridge** forwards data and remote frames, not error frames or a socket's
  own transmissions; it runs for a bounded `--duration`.
- **Databases:** DBC, KCD and ARXML (the CAN part of AUTOSAR 4 system
  descriptions) in; DBC, KCD and JSON out. No ARXML writer. What KCD has no
  place for (attributes, value tables, initial values, node comments, signal
  groups) is dropped and printed as warnings; extended multiplexing cannot be
  written as KCD (exit 8). Environment variables and other DBC constructs
  go-canbus ignores are reported as warnings (see go-canbus LIMITATIONS.md).
- **Traces:** candump logs keep microseconds; pcap does not keep interface
  names or the own-message mark; ASC and BLF keep channel numbers, not
  interface names, and no error class (`trace convert` reports each loss);
  JSON Lines is lossless. ASC, BLF and TRC times are zone-less local time
  (`TZ`). pcapng keeps interface names but, like pcap, not the
  own-message mark. MF4 is not supported. Viewing CAN XL in pcap needs
  Wireshark 4.4 or later.
- **CAN XL** needs a SocketCAN interface with the CAN XL MTU (Linux ≥ 6.2;
  VCIDs ≥ 6.9) or the virtual bus. It is carried by `xl send`, `xl
  monitor`, `record --xl`, `replay --xl`, `trace convert` and `trace info`;
  not by `monitor`, `stats`, `bridge`, `inject`, `simulate`, the ISO-TP or
  `dbc` commands, nor slcan adapters. Only candump logs, JSON Lines, pcap
  and pcapng hold CAN XL; `trace convert` reports CAN XL frames it cannot
  write.
- `simulate` generates values from patterns (counter, random, initial); it does
  not model ECU behaviour or react to traffic.
- No device discovery or fingerprinting: a CAN identifier does not identify a
  device.

// SPDX-License-Identifier: MIT

// Package e2e drives the built canbusctl binary end to end: trace format
// chains and database decoding everywhere; on Linux with vcan0 and vcan1,
// separate processes exchanging frames and ISO-TP messages, the kernel's
// ISO-TP, and the chaos bridge under faults.
package e2e

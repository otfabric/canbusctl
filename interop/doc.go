// SPDX-License-Identifier: MIT

// Package interop tests the canbusctl binary against the independent CAN
// implementations of the canbus-interop images (can-utils and the Linux
// kernel, python-can, python-can-isotp, cantools, canmatrix). The tests
// carry the interop build tag and need Docker and a vcan interface:
//
//	go test -tags interop -p 1 ./interop
//
// A capability an image lacks is a skip, recorded as such in the report
// (CANBUS_INTEROP_REPORT), never a pass.
package interop

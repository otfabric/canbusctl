// SPDX-License-Identifier: MIT

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/otfabric/go-canbus/socketcan/link"
)

func fakeInterfaces() []link.Interface {
	st := link.StateErrorPassive
	r := uint32(100)
	return []link.Interface{
		{Name: "vcan0", Index: 3, Kind: "vcan", Up: true, OperState: "UNKNOWN", MTU: 72, TxQueueLen: 1000},
		{Name: "can0", Index: 4, Kind: "can", Up: true, OperState: "UP", MTU: 72, State: &st,
			BitTiming: &link.BitTiming{Bitrate: 500000, SamplePoint: 875}, DataBitTiming: &link.BitTiming{Bitrate: 2000000, SamplePoint: 750},
			ClockHz: 80000000, CtrlMode: link.ModeFD, CtrlModeSupported: link.ModeFD | link.ModeLoopback, RestartMs: &r,
			ErrorCounters: &link.ErrorCounters{TX: 1, RX: 130}, DeviceStats: &link.DeviceStats{BusOff: 2}},
		{Name: "slc0", MTU: 16, BitTiming: &link.BitTiming{Bitrate: 250000}},
	}
}

func TestInterfaceResults(t *testing.T) {
	r := interfacesResult{Interfaces: fakeInterfaces()}
	text := r.Text()
	for _, w := range []string{"vcan0", "UP/ERROR-PASSIVE", "500000/2000000", "slc0", "?", "250000"} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
	if len(r.Rows()) != 3 || r.Headers()[0] != "NAME" || r.Rows()[2][3] != "16" {
		t.Fatal(r.Rows())
	}
	if (interfacesResult{}).Text() != "no CAN interfaces" {
		t.Fatal("empty")
	}
	info := infoResult{fakeInterfaces()[1]}.Text()
	for _, w := range []string{"bitrate     500000, sample point 87.5%", "dbitrate    2000000", "clock       80000000 Hz", "restart-ms  100", "errors      tx 1 rx 130", "bus-off 2", "ctrlmode    FD"} {
		if !strings.Contains(info, w) {
			t.Errorf("info lacks %q:\n%s", w, info)
		}
	}
	if s := (actionResult{Interface: "can0", Action: "up"}).Text(); s != "can0: up done" {
		t.Fatal(s)
	}
	st := statsResult{Interface: "can0", Link: &fakeInterfaces()[1]}
	if !strings.Contains(st.Text(), "can0 (index 4") || len(st.Rows()) != 0 {
		t.Fatal(st.Text())
	}
}

func TestExecute(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"canbusctl", "version"}
	if Execute() != 0 {
		t.Fatal("Execute")
	}
}

func TestSimulateFloats(t *testing.T) {
	r := run(t, "", "simulate", "--dbc", "testdata/floats.dbc", "--max-frames", "3", "--default-cycle", "1ms")
	want(t, r, 0, "F32=1", "F64=2", "simulated 3")
	want(t, run(t, "", "simulate", "--dbc", "testdata/floats.dbc", "--max-frames", "1", "--pattern", "initial", "--default-cycle", "1ms"), 0, "I=5")
}

// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
	"github.com/cB-Sigma/tuyalan/contrib"
)

func openDevice(t *testing.T, d *sim.Device, onChange func(map[string]any)) *tuyalan.Device {
	t.Helper()
	dev := tuyalan.NewDevice(tuyalan.DeviceOptions{Addr: d.Addr(), Version: d.Config().Version, DevID: d.Config().ID,
		LocalKey: []byte(key), Timeout: 2 * time.Second, Heartbeat: time.Second, Reconnect: 100 * time.Millisecond, OnChange: onChange})
	if err := dev.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dev.Close() })
	return dev
}

// must panics on a helper error; the helpers here are given valid values.
func must(m map[string]any, err error) map[string]any {
	if err != nil {
		panic(err)
	}
	return m
}

// The fan and air conditioner helpers set the simulated devices through
// the persistent client, and read them back.
func TestContribFanAndClimate(t *testing.T) {
	fan := sim.NewFan()
	d := startWith(t, "3.4", fan)
	dev := openDevice(t, d, nil)
	f := contrib.DefaultFan
	for _, m := range []map[string]any{f.SetOn(true), must(f.SetMode("nature")), must(f.SetSpeed(4)), f.SetOscillation(true), must(f.SetTimer("2h"))} {
		if err := dev.Set(m); err != nil {
			t.Fatal(err)
		}
	}
	dps, err := dev.Status()
	if err != nil {
		t.Fatal(err)
	}
	if st := f.State(dps); !st.On || st.Mode != "nature" || st.Speed != 4 || !st.Oscillation || st.Timer != "2h" {
		t.Errorf("fan %+v", st)
	}

	ac := sim.NewClimate()
	d2 := startWith(t, "3.5", ac)
	dev2 := openDevice(t, d2, nil)
	c := contrib.DefaultClimate
	if err := dev2.Set(contrib.Merge(c.SetOn(true), must(c.SetMode("hot")), c.SetTargetTemperature(26))); err != nil {
		t.Fatal(err)
	}
	dps, _ = dev2.Status()
	if st := c.State(dps); !st.On || st.Mode != "hot" || st.TargetTemp != 26 || st.RoomTemp != 27 {
		t.Errorf("climate %+v", st)
	}
}

// IR blasters of both control types: study, a learned code pushed by the
// blaster, and sending it back.
func TestContribIRBlaster(t *testing.T) {
	for _, typ := range []int{1, 2} {
		t.Run(map[int]string{1: "type1", 2: "type2"}[typ], func(t *testing.T) {
			ir := sim.NewIRBlaster(typ)
			d := startWith(t, "3.3", ir)
			learned := make(chan string, 4)
			dev := openDevice(t, d, func(dps map[string]any) {
				if c, ok := contrib.IRLearned(dps); ok && c != "" {
					learned <- c
				}
			})
			dps, err := dev.Status()
			if err != nil {
				t.Fatal(err)
			}
			if got := contrib.IRControlType(dps); got != typ {
				t.Fatalf("control type %d, want %d", got, typ)
			}
			if err := dev.Set(must(contrib.IRCommand(typ, "study"))); err != nil {
				t.Fatal(err)
			}
			if !ir.Studying() {
				t.Fatal("not studying")
			}
			code, _ := tuyalan.IRPulsesToBase64(tuyalan.IRNECToPulses(4, 8))
			d.Push(ir.Learn(code))
			select {
			case c := <-learned:
				if c != code {
					t.Errorf("learned %s", c)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("no learned code pushed")
			}
			if err := dev.Set(must(contrib.IRCommand(typ, "study_exit"))); err != nil {
				t.Fatal(err)
			}
			head, k, _ := tuyalan.IRPulsesToHeadKey(tuyalan.IRNECToPulses(4, 8), 0.1, 38)
			if err := dev.Set(must(contrib.IRSendHeadKey(typ, head, k))); err != nil {
				t.Fatal(err)
			}
			cmds := ir.Commands()
			last := cmds[len(cmds)-1]
			if typ == 1 {
				if last["control"] != "send_ir" || last["head"] != head || last["key1"] != "0"+k {
					t.Errorf("command %v", last)
				}
			} else if last["1"] != "send_ir" || last["3"] != head || last["4"] != k {
				t.Errorf("command %v", last)
			}
			if ir.Studying() {
				t.Error("still studying")
			}
		})
	}
}

// The thermostat's state, its remote sensor list and a setpoint change.
func TestContribThermostat(t *testing.T) {
	th := sim.NewThermostat()
	d := startWith(t, "3.3", th)
	dev := openDevice(t, d, nil)
	dps, err := dev.Status()
	if err != nil {
		t.Fatal(err)
	}
	st := contrib.ThermostatStateOf(dps)
	if st.HighResolution == nil || *st.HighResolution || st.Mode != "cool" || *st.TemperatureC != 21.5 || *st.CoolingC != 24 {
		t.Fatalf("state %+v", st)
	}
	l, err := contrib.ParseThermostatSensors(dps["122"].(string))
	if err != nil || len(l.Sensors) != 1 || l.Sensors[0].Name() != "Hall" || l.Sensors[0].Temperature() != 20.49 || l.Sensors[0].Battery != 87 {
		t.Fatalf("sensors %+v %v", l, err)
	}
	set, err := st.Setter().SetSetpoint(23, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := dev.Set(set); err != nil {
		t.Fatal(err)
	}
	dps, _ = dev.Status()
	if st := contrib.ThermostatStateOf(dps); *st.CoolingC != 23 {
		t.Errorf("cooling setpoint %v", *st.CoolingC)
	}
	// Rename the sensor and send the list back.
	l.Sensors[0].SetName("Landing")
	if err := dev.Set(map[string]any{"122": l.B64()}); err != nil {
		t.Fatal(err)
	}
	dps, _ = dev.Status()
	l2, _ := contrib.ParseThermostatSensors(dps["122"].(string))
	if b, _ := json.Marshal(l2.Sensors[0].Name()); string(b) != `"Landing"` {
		t.Errorf("renamed %s", b)
	}
}

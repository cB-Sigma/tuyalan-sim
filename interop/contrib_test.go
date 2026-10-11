// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
	"github.com/cB-Sigma/tuyalan/contrib"
)

// tinytuya's IRRemoteControlDevice detects the simulated blaster's control
// type, sends a learned code and a head/key (decoded by the simulator into
// the commands the contrib helpers build), and learns a button the
// simulator pushes during its study.
func TestTinytuyaIRBlasterAgainstSimulator(t *testing.T) {
	needDocker(t)
	for _, typ := range []int{1, 2} {
		t.Run(fmt.Sprint("type", typ), func(t *testing.T) {
			ir := sim.NewIRBlaster(typ)
			id := fmt.Sprintf("bfsimirblaster%d01", typ)
			d, port := startSim(t, "3.3", id, ir)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "client.py", "irblaster", "host.docker.internal", port, "3.3", id, localKey)
			typEv := event(t, p.expect(60*time.Second, "type"))
			if typEv["type"] != float64(typ) {
				t.Fatalf("tinytuya detected %v, want %d", typEv, typ)
			}
			sent := event(t, p.expect(15*time.Second, "sent"))
			code, _ := tuyalan.IRPulsesToBase64(tuyalan.IRNECToPulses(4, 8))
			head, key, _ := tuyalan.IRPulsesToHeadKey(tuyalan.IRNECToPulses(4, 8), 0.1, 38)
			if sent["head"] != head || sent["key"] != key {
				t.Errorf("tinytuya's head/key %v, ours %s %s", sent, head, key)
			}
			// Wait for the study, then press the button.
			deadline := time.Now().Add(20 * time.Second)
			for !ir.Studying() && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			if !ir.Studying() {
				t.Fatalf("no study started; commands %v", ir.Commands())
			}
			d.Push(ir.Learn(code))
			got := event(t, p.expect(20*time.Second, "learned"))
			if got["code"] != code {
				t.Errorf("tinytuya learned %v", got)
			}
			p.expect(10*time.Second, "done")

			// The commands tinytuya sent are the contrib helpers' commands.
			want := []map[string]any{}
			for _, f := range []func() (map[string]any, error){
				func() (map[string]any, error) { return contrib.IRSendCode(typ, code) },
				func() (map[string]any, error) { return contrib.IRSendHeadKey(typ, head, key) },
			} {
				m, err := f()
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, m)
			}
			var found int
			for _, c := range ir.Commands() {
				for _, w := range want {
					if typ == 1 {
						if sameJSON(t, w[contrib.IRType1Command].(string), c) {
							found++
						}
					} else if reflect.DeepEqual(normalise(w), c) {
						found++
					}
				}
			}
			if found != 2 {
				t.Errorf("found %d of the 2 sends among %v", found, ir.Commands())
			}
		})
	}
}

// normalise turns a helper's map into what the simulator decodes (JSON
// numbers as float64).
func normalise(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if n, ok := v.(int); ok {
			v = float64(n)
		}
		out[k] = v
	}
	return out
}

func sameJSON(t *testing.T, s string, decoded map[string]any) bool {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(m, decoded)
}

// tinytuya's FloorFanDevice sets the simulated fan; the controls are the
// contrib helpers' controls.
func TestTinytuyaFanAgainstSimulator(t *testing.T) {
	needDocker(t)
	fan := sim.NewFan()
	id := "bfsimfloorfan001"
	_, port := startSim(t, "3.4", id, fan)
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "fan", "host.docker.internal", port, "3.4", id, localKey)
	st := event(t, p.expect(60*time.Second, "fan"))
	if st["Mode"] != "nature" || st["Speed"] != 3.0 || st["Timer"] != "4h" || st["Power"] != true || st["Oscillation"] != true {
		t.Errorf("tinytuya status %v", st)
	}
	p.expect(10*time.Second, "done")
	f := contrib.DefaultFan
	mode, _ := f.SetMode("nature")
	speed, _ := f.SetSpeed(3)
	timer, _ := f.SetTimer("4h")
	want := []map[string]any{f.SetOn(true), mode, speed, f.SetOscillation(true), timer}
	got := fan.Controls()
	if len(got) != len(want) {
		t.Fatalf("controls %v", got)
	}
	for i := range want {
		if !reflect.DeepEqual(normalise(want[i]), got[i]) {
			t.Errorf("control %d: tinytuya %v, contrib %v", i, got[i], want[i])
		}
	}
}

// tinytuya's ThermostatDevice reads the simulated thermostat and its
// remote sensor list, sets a cooling setpoint and renames the sensor; the
// contrib helpers read and write the same values.
func TestTinytuyaThermostatAgainstSimulator(t *testing.T) {
	needDocker(t)
	th := sim.NewThermostat()
	id := "bfsimthermostat1"
	_, port := startSim(t, "3.3", id, th)
	// The state before tinytuya changes it.
	st := contrib.ThermostatStateOf(th.Read(nil))
	sensors, _ := contrib.ParseThermostatSensors(th.Read(nil)["122"].(string))
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "thermostat", "host.docker.internal", port, "3.3", id, localKey)
	ev := event(t, p.expect(60*time.Second, "thermostat"))
	if ev["high_resolution"] != false || ev["cooling_c"] != *st.CoolingC || ev["temperature_c"] != *st.TemperatureC {
		t.Errorf("tinytuya read %v, contrib %+v", ev, st)
	}
	ts := ev["sensors"].([]any)
	if len(ts) != 1 {
		t.Fatalf("tinytuya sensors %v", ts)
	}
	s0 := ts[0].(map[string]any)
	if s0["id"] != sensors.Sensors[0].ID() || s0["name"] != sensors.Sensors[0].Name() || s0["temperature"] != sensors.Sensors[0].Temperature() ||
		s0["battery"] != float64(sensors.Sensors[0].Battery) {
		t.Errorf("tinytuya sensor %v, contrib %+v", s0, sensors.Sensors[0])
	}
	after := event(t, p.expect(20*time.Second, "after"))
	p.expect(10*time.Second, "done")
	if after["cooling_c"] != 23.0 || fmt.Sprint(after["names"]) != "[Landing]" {
		t.Errorf("tinytuya after %v", after)
	}
	ctl := th.Controls()
	if len(ctl) != 2 {
		t.Fatalf("controls %v", ctl)
	}
	set, _ := st.Setter().SetCoolSetpoint(23, "")
	if !reflect.DeepEqual(normalise(set), ctl[0]) {
		t.Errorf("setpoint: tinytuya %v, contrib %v", ctl[0], set)
	}
	sensors.Sensors[0].SetName("Landing")
	if ctl[1]["122"] != sensors.B64() {
		t.Errorf("renamed list: tinytuya %v, contrib %s", ctl[1], sensors.B64())
	}
}

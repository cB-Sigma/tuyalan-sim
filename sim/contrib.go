// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"encoding/json"
	"sync"

	"github.com/cB-Sigma/tuyalan/contrib"
)

// More device types, on the data points the tuyalan contrib helpers (and
// tinytuya's Contrib classes) use.

// NewFan returns a floor fan: 1 power (off), 2 mode "normal", 3 speed 1,
// 5 oscillation off, 22 timer "cancel".
func NewFan() *Memory {
	f := contrib.DefaultFan
	return NewMemory(map[string]any{f.Power: false, f.Mode: "normal", f.Speed: 1.0, f.Oscillation: false, f.Timer: "cancel"})
}

// NewClimate returns a portable air conditioner, off, cooling to 24 in a
// 27-degree room.
func NewClimate() *Memory {
	c := contrib.DefaultClimate
	return NewMemory(map[string]any{c.Power: false, c.SetTemp: 24.0, c.CurTemp: 27.0, c.Mode: "cold", c.Fan: "auto",
		c.TempUnit: "c", c.Timer: 0.0, c.SleepPreset: false, c.Swing: false, c.Operating: "cold"})
}

// NewThermostat returns a "low resolution" 24 V thermostat in cool mode
// at 21.5 C with one remote sensor ("Hall", 20.49 C) on data point 122.
func NewThermostat() *Memory {
	var s contrib.ThermostatSensor
	s.RawID = 0x1a2b3c4d
	s.SetName("Hall")
	s.Enabled, s.Occupied, s.Online, s.Averaging = true, true, true, true
	s.RawTemperatureUsed, s.RawTemperature = 2050, 2049
	s.Participation, s.Battery, s.Firmware = contrib.ParticipateWake|contrib.ParticipateHome, 87, 12
	sensors := contrib.ThermostatSensorList{StatedCount: 1, Sensors: []contrib.ThermostatSensor{s}}
	return NewMemory(map[string]any{
		contrib.ThermoMode: "cool", contrib.ThermoSetpointC: 2150.0, contrib.ThermoSetpointF: 71.0,
		contrib.ThermoCoolC: 24.0, contrib.ThermoCoolF: 75.0, contrib.ThermoHeatC: 20.0, contrib.ThermoHeatF: 68.0,
		contrib.ThermoUnits: "c", contrib.ThermoTemperatureC: 2150.0, contrib.ThermoTemperatureF: 71.0,
		contrib.ThermoTempCorrection: 0.0, contrib.ThermoHumidity: 45.0, contrib.ThermoFault: 0.0, contrib.ThermoSystemType: "4",
		contrib.ThermoFan: "auto", contrib.ThermoHome: true, contrib.ThermoScheduleEnabled: true,
		contrib.ThermoHold: "followschedule", contrib.ThermoFanRunTime: 20.0, contrib.ThermoSystem: "alloff",
		contrib.ThermoSensorLists[0]: sensors.B64(),
	})
}

// IRBlaster is an IR blaster of control type 1 (commands as JSON text on
// data point 201, learned codes on 202) or 2 (data points 1 to 13, learned
// codes on 2). It records the commands it gets; Learn gives the data
// points to push when a button is learned.
type IRBlaster struct {
	*Memory
	controlType int

	mu       sync.Mutex
	commands []map[string]any
	studying bool
}

// NewIRBlaster returns an IR blaster of control type 1 or 2.
func NewIRBlaster(controlType int) *IRBlaster {
	dps := map[string]any{contrib.IRType1Command: "", contrib.IRType1Learned: ""}
	if controlType == 2 {
		dps = map[string]any{contrib.IRType2Mode: "study_exit", contrib.IRType2Learned: "", contrib.IRType2Head: "",
			contrib.IRType2Key: "", contrib.IRType2Study: "", contrib.IRType2Code: 0.0}
	}
	return &IRBlaster{Memory: NewMemory(dps), controlType: controlType}
}

// Write implements Backend: it records the command (the decoded JSON of
// data point 201 for type 1, the data points for type 2) and follows the
// study mode.
func (b *IRBlaster) Write(dps map[string]any) (map[string]any, bool) {
	cmd := map[string]any{}
	if b.controlType == 1 {
		v, present := dps[contrib.IRType1Command]
		if !present {
			return b.Memory.Write(dps) // other data points are not commands
		}
		s, ok := v.(string)
		if !ok || json.Unmarshal([]byte(s), &cmd) != nil {
			return nil, false
		}
	} else {
		for k, v := range dps {
			cmd[k] = v
		}
	}
	mode, _ := cmd["control"].(string)
	if b.controlType == 2 {
		mode, _ = cmd[contrib.IRType2Mode].(string)
	}
	b.mu.Lock()
	b.commands = append(b.commands, cmd)
	switch mode {
	case "study":
		b.studying = true
	case "study_exit":
		b.studying = false
	}
	b.mu.Unlock()
	return b.Memory.Write(dps)
}

// Commands returns the commands received, in order.
func (b *IRBlaster) Commands() []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]map[string]any(nil), b.commands...)
}

// Studying reports whether a study (button learning) is running.
func (b *IRBlaster) Studying() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.studying
}

// Learn stores a learned code and returns the data points to push with
// Device.Push, as the blaster reports a button pressed during a study.
func (b *IRBlaster) Learn(code string) map[string]any {
	dp := contrib.IRType1Learned
	if b.controlType == 2 {
		dp = contrib.IRType2Learned
	}
	b.Memory.Set(dp, code)
	return map[string]any{dp: code}
}

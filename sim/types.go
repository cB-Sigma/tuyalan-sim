// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"github.com/cB-Sigma/tuyalan"
)

// Device types: backends holding the data points of common devices, in
// the layouts the tuyalan helpers (and tinytuya's device classes) use.

// NewPlug returns the data points of a metering plug: 1 switch (off), 9
// countdown in seconds, 18 current in mA, 19 power in 0.1 W, 20 voltage
// in 0.1 V.
func NewPlug() *Memory {
	return NewMemory(map[string]any{"1": false, "9": 0.0, "18": 0.0, "19": 0.0, "20": 2301.0})
}

// NewBulb returns the data points of a bulb of layout l (tuyalan.BulbV1,
// BulbV2 or BulbDimmer), off, in white light at the lowest brightness.
func NewBulb(l tuyalan.BulbLayout) *Memory {
	dps := map[string]any{l.Switch: false}
	if l.Brightness != "" {
		dps[l.Brightness] = float64(l.Min)
	}
	if l.ColourTemp != "" {
		dps[l.ColourTemp] = 0.0
	}
	if l.Mode != "" {
		dps[l.Mode] = tuyalan.ModeWhite
	}
	if l.Colour != "" {
		dps[l.Colour] = tuyalan.ColourHexRGB(255, 0, 0, l.Format)
	}
	if l.Name == "B" {
		// The newer layout also has scene data and a countdown.
		dps[l.Scene] = "000e0d0000000000000000c80000"
		dps[l.Timer] = 0.0
	}
	return NewMemory(dps)
}

// Cover is a curtain motor on tuyalan.DefaultCover's data points: 1
// control ("open", "close", "stop", "continue"), 2 position to go to and
// 3 position reached, in percent (100 open). It moves at once: "open"
// reaches 100, "close" 0, a position set on 2 is reached on 3.
type Cover struct {
	*Memory
}

// NewCover returns a stopped, closed cover.
func NewCover() *Cover {
	return &Cover{Memory: NewMemory(map[string]any{"1": "stop", "2": 0.0, "3": 0.0})}
}

// Write implements Backend. A control value that is not one of scheme
// 1's, or a position outside 0 to 100, refuses the whole control.
func (c *Cover) Write(dps map[string]any) (map[string]any, bool) {
	if v, ok := dps["1"]; ok {
		switch v {
		case "open", "close", "stop", "continue":
		default:
			return nil, false
		}
	}
	if v, ok := dps["2"]; ok {
		f, num := v.(float64)
		if !num || f < 0 || f > 100 {
			return nil, false
		}
	}
	written, ok := c.Memory.Write(dps)
	if !ok {
		return written, ok
	}
	changed := map[string]any{}
	for k, v := range written {
		changed[k] = v
	}
	target, moving := -1.0, false
	switch changed["1"] {
	case "open":
		target, moving = 100, true
	case "close":
		target, moving = 0, true
	}
	if f, ok := changed["2"].(float64); ok {
		target, moving = f, true
	}
	if moving {
		c.Memory.Set("2", target)
		c.Memory.Set("3", target)
		changed["2"], changed["3"] = target, target
	}
	return changed, true
}

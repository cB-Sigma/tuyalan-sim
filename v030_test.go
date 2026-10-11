// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

const (
	subA = "a4c1380000000001"
	subB = "a4c1380000000002"
)

// SubdevQuery reads the gateway's online/offline sub-device lists (3.4 and
// 3.5 answer; 3.3 does not).
func TestSubdevQueryAgainstSimulator(t *testing.T) {
	for _, v := range []string{"3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			d := sim.New(sim.Config{ID: "bfsimgw" + v, LocalKey: key, Version: v}, sim.NewMemory(map[string]any{"1": true}))
			if _, err := d.Start("127.0.0.1:0"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(d.Stop)
			d.AddSub(subA, sim.NewPlug())
			d.AddSub(subB, sim.NewPlug())
			d.SetSubOffline(subB, true)
			c := dial(t, d)
			if err := c.SubdevQuery(); err != nil {
				t.Fatal(err)
			}
			_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
			for {
				m, err := c.Recv()
				if err != nil {
					t.Fatalf("waiting for the sub-device list: %v", err)
				}
				s, ok := tuyalan.ParseSubdevStatus(m.Payload)
				if !ok {
					continue
				}
				if !reflect.DeepEqual(s.Online, []string{subA}) || !reflect.DeepEqual(s.Offline, []string{subB}) {
					t.Errorf("status %+v", s)
				}
				return
			}
		})
	}
}

// A 3.3 gateway does not answer the sub-device query.
func TestSubdevQueryLegacyNoAnswer(t *testing.T) {
	d := startWith(t, "3.3", sim.NewMemory(map[string]any{"1": true}))
	d.AddSub(subA, sim.NewPlug())
	c := dial(t, d)
	if err := c.SubdevQuery(); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	if m, err := c.Recv(); err == nil {
		if _, ok := tuyalan.ParseSubdevStatus(m.Payload); ok {
			t.Errorf("3.3 gateway answered the sub-device query: %s", m.Payload)
		}
	}
}

// DetectAvailableDPS finds the data points a device answers by probing the
// ranges a device22 brute force uses.
func TestDetectAvailableDPSAgainstSimulator(t *testing.T) {
	mem := sim.NewMemory(map[string]any{"1": false, "9": 0.0, "18": 0.0, "19": 0.0, "20": 2301.0})
	d := startWith(t, "3.3", mem)
	c := dial(t, d)
	got, err := c.DetectAvailableDPS(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"1", "9", "18", "19", "20"}) {
		t.Errorf("detected %v", got)
	}
}

// IdentifyWithKeys tries a list of keys and returns the one that works.
func TestIdentifyWithKeysAgainstSimulator(t *testing.T) {
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			d := startWith(t, v, sim.NewMemory(map[string]any{"1": true}))
			keys := [][]byte{[]byte("ffffffffffffffff"), []byte("eeeeeeeeeeeeeeee"), []byte(key)}
			id, idx, err := tuyalan.IdentifyWithKeys(context.Background(), d.Addr(), d.Config().ID, keys, 2*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if idx != 2 {
				t.Errorf("key index %d, want 2", idx)
			}
			if v != "3.1" && id.Version != v {
				t.Errorf("version %q, want %q", id.Version, v)
			}
		})
	}
}

// IdentifyWithKeys fails cleanly when no key matches a 3.4 or 3.5 device.
func TestIdentifyWithKeysNoMatch(t *testing.T) {
	d := startWith(t, "3.5", sim.NewMemory(map[string]any{"1": true}))
	keys := [][]byte{[]byte("ffffffffffffffff"), []byte("eeeeeeeeeeeeeeee")}
	if _, _, err := tuyalan.IdentifyWithKeys(context.Background(), d.Addr(), d.Config().ID, keys, time.Second); err == nil {
		t.Error("identified with wrong keys")
	}
}

// Status returns the query's own answer even when a set's push is still
// on its way (found when a status right after a set returned the push).
func TestDeviceStatusIsTheAnswer(t *testing.T) {
	for _, v := range []string{"3.3", "3.5"} {
		mem := sim.NewMemory(map[string]any{"1": false, "2": 100.0, "3": "a"})
		d := startWith(t, v, mem)
		dev := tuyalan.NewDevice(tuyalan.DeviceOptions{Addr: d.Addr(), Version: v, DevID: d.Config().ID, LocalKey: []byte(key), Timeout: 2 * time.Second})
		if err := dev.Open(context.Background()); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 20; i++ {
			if err := dev.Set(map[string]any{"2": float64(i)}); err != nil {
				t.Fatal(err)
			}
			st, err := dev.Status()
			if err != nil {
				t.Fatal(err)
			}
			if len(st) != 3 || st["2"] != float64(i) {
				t.Fatalf("%s: status after set %d: %v", v, i, st)
			}
		}
		dev.Close()
	}
}

// A device22 device is read with QueryDPS (null-value CONTROL_NEW).
func TestDeviceDevice22(t *testing.T) {
	mem := sim.NewMemory(map[string]any{"1": true, "2": 5.0})
	d := sim.New(sim.Config{ID: "bfsimdev22", LocalKey: key, Version: "3.3", Device22: true}, mem)
	if _, err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	dev := tuyalan.NewDevice(tuyalan.DeviceOptions{Addr: d.Addr(), Version: "3.3", DevID: "bfsimdev22", LocalKey: []byte(key),
		Timeout: 2 * time.Second, QueryDPS: []string{"1", "2"}})
	if err := dev.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	for i := 0; i < 5; i++ {
		if err := dev.Set(map[string]any{"2": float64(10 + i)}); err != nil {
			t.Fatal(err)
		}
		st, err := dev.Status()
		if err != nil || st["1"] != true || st["2"] != float64(10+i) {
			t.Fatalf("status %v %v", st, err)
		}
	}
}

// The persistent Device reads, sets, caches, hears pushes and reconnects.
func TestDeviceAgainstSimulator(t *testing.T) {
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			mem := sim.NewMemory(map[string]any{"1": false, "2": 100.0})
			d := startWith(t, v, mem)

			var mu sync.Mutex
			var pushes []map[string]any
			states := make(chan bool, 8)
			dev := tuyalan.NewDevice(tuyalan.DeviceOptions{
				Addr: d.Addr(), Version: v, DevID: d.Config().ID, LocalKey: []byte(key),
				Timeout: 2 * time.Second, Heartbeat: 200 * time.Millisecond, Reconnect: 100 * time.Millisecond,
				OnChange: func(dps map[string]any) {
					mu.Lock()
					pushes = append(pushes, dps)
					mu.Unlock()
				},
				OnState: func(connected bool, _ error) {
					select {
					case states <- connected:
					default:
					}
				},
			})
			if err := dev.Open(context.Background()); err != nil {
				t.Fatal(err)
			}
			defer dev.Close()

			st, err := dev.Status()
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			if st["1"] != false || st["2"] != 100.0 {
				t.Errorf("status %v", st)
			}
			if err := dev.Set(map[string]any{"1": true}); err != nil {
				t.Fatalf("set: %v", err)
			}
			// The push of the change reaches OnChange and the cache.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if dev.Cache()["1"] == true {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if dev.Cache()["1"] != true {
				t.Errorf("cache %v", dev.Cache())
			}
			mu.Lock()
			gotPush := false
			for _, p := range pushes {
				if p["1"] == true {
					gotPush = true
				}
			}
			mu.Unlock()
			if !gotPush {
				t.Error("no push of the change reached OnChange")
			}

			// A device started at the device reaches OnChange.
			mem.Set("2", 250.0)
			d.Push(map[string]any{"2": 250.0})
			deadline = time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if dev.Cache()["2"] == 250.0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if dev.Cache()["2"] != 250.0 {
				t.Errorf("cache after device push %v", dev.Cache())
			}

			// Drop the network: the Device reconnects on its own.
			d.SetDown(true)
			time.Sleep(300 * time.Millisecond)
			d.SetDown(false)
			deadline = time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) && !dev.Connected() {
				time.Sleep(50 * time.Millisecond)
			}
			if !dev.Connected() {
				t.Fatal("the device did not reconnect")
			}
			if _, err := dev.Status(); err != nil {
				t.Errorf("status after reconnect: %v", err)
			}
		})
	}
}

// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop runs tinytuya's client (peer/client.py, in a container)
// against the simulated device of every protocol version, and tinytuya's
// announcement decoder against the simulator's announcements. The tests
// need Docker and are skipped unless SIGMA_TUYALAN_INTEROP=1.
package interop

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

const localKey = "0123456789abcdef"

// event splits a peer line "<name> <json>" and decodes the JSON.
func event(t *testing.T, line string) map[string]any {
	t.Helper()
	_, js, _ := strings.Cut(line, " ")
	var m map[string]any
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("peer line %q: %v", line, err)
	}
	return m
}

func dpsOf(m map[string]any) map[string]any {
	d, _ := m["dps"].(map[string]any)
	return d
}

// tinytuya reads the simulated device, switches it on, sets a value, sends
// a heartbeat and receives a push, for protocol 3.1 to 3.5.
func TestTinytuyaAgainstSimulator(t *testing.T) {
	needDocker(t)
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			mem := sim.NewMemory(map[string]any{"1": false, "2": 100.0})
			id := "bfsimplug" + strings.ReplaceAll(v, ".", "") + "00001"
			d := sim.New(sim.Config{ID: id, LocalKey: localKey, Version: v}, mem)
			addr, err := d.Start("127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer d.Stop()
			_, port, _ := net.SplitHostPort(addr)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "client.py", "session", "host.docker.internal", port, v, id, localKey)

			st := event(t, p.expect(60*time.Second, "status"))
			if dps := dpsOf(st); dps["1"] != false || dps["2"] != 100.0 {
				t.Errorf("tinytuya status %v", st)
			}
			p.expect(10*time.Second, "turn_on")
			p.expect(10*time.Second, "set_value")
			p.expect(10*time.Second, "heartbeat")
			st2 := event(t, p.expect(10*time.Second, "status2"))
			if dps := dpsOf(st2); dps["1"] != true || dps["2"] != 123.0 {
				t.Errorf("tinytuya status after the changes %v", st2)
			}
			if ctl := mem.Controls(); len(ctl) != 2 || ctl[0]["1"] != true || ctl[1]["2"] != 123.0 {
				t.Errorf("controls the simulator received %v", ctl)
			}
			p.expect(10*time.Second, "waiting")
			time.Sleep(200 * time.Millisecond)
			d.Push(map[string]any{"2": 77.0})
			push := event(t, p.expect(15*time.Second, "push"))
			if dps := dpsOf(push); dps["2"] != 77.0 {
				t.Errorf("tinytuya push %v", push)
			}
			p.expect(10*time.Second, "done")
		})
	}
}

// freeUDPPort returns a UDP port free on the loopback interface.
func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// tinytuya's decoder reads the simulator's announcements of every version
// on the port each version uses.
func TestTinytuyaHearsSimulator(t *testing.T) {
	needDocker(t)
	ports := map[int]int{6666: freeUDPPort(t), 6667: freeUDPPort(t), 7000: freeUDPPort(t)}
	var args []string
	for in, out := range ports {
		args = append(args, "-p", fmt.Sprintf("127.0.0.1:%d:%d/udp", out, in))
	}
	args = append(args, image, "python", "-u", "client.py", "listen")
	p := startPeer(t, args...)
	p.expect(60*time.Second, "listening")
	portOf := map[string]int{"3.1": 6666, "3.2": 6667, "3.3": 6667, "3.4": 6667, "3.5": 7000}
	for _, v := range tuyalan.AllVersions {
		id := "bfhear" + strings.ReplaceAll(v, ".", "") + "000001"
		d := sim.New(sim.Config{ID: id, LocalKey: localKey, Version: v, ProductKey: "keysim"}, sim.NewMemory(nil))
		if _, err := d.Start("127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		defer d.Stop()
		to := net.JoinHostPort("127.0.0.1", strconv.Itoa(ports[portOf[v]]))
		var heard map[string]any
		for try := 0; try < 10 && heard == nil; try++ {
			if err := d.Broadcast(to); err != nil {
				t.Fatal(err)
			}
			select {
			case l := <-p.lines:
				if strings.HasPrefix(l, "heard ") && strings.Contains(l, id) {
					heard = event(t, l)
				} else if strings.HasPrefix(l, "undecodable") {
					t.Fatalf("%s: tinytuya could not decode: %s", v, l)
				}
			case <-time.After(500 * time.Millisecond):
			}
		}
		if heard == nil {
			t.Fatalf("%s: tinytuya heard nothing", v)
		}
		js, _ := heard["json"].(map[string]any)
		if js["gwId"] != id || js["version"] != v || js["productKey"] != "keysim" || int(heard["port"].(float64)) != portOf[v] {
			t.Errorf("%s: tinytuya decoded %v", v, heard)
		}
	}
}

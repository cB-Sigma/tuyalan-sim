// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

func startSim(t *testing.T, v, id string, b sim.Backend) (*sim.Device, string) {
	t.Helper()
	d := sim.New(sim.Config{ID: id, LocalKey: localKey, Version: v, ProductKey: "keysim"}, b)
	addr, err := d.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	_, port, _ := net.SplitHostPort(addr)
	return d, port
}

// tinytuya's child devices (cid, parent) read, switch and refresh a plug
// sub-device of the simulated gateway and read a bulb sub-device, for
// every version.
func TestTinytuyaGatewayAgainstSimulator(t *testing.T) {
	needDocker(t)
	const cid1, cid2 = "a4c1380000000001", "a4c1380000000002"
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			gw := sim.NewMemory(map[string]any{"1": true})
			id := "bfsimgw" + strings.ReplaceAll(v, ".", "") + "0000001"
			d, port := startSim(t, v, id, gw)
			plug, bulb := sim.NewPlug(), sim.NewBulb(tuyalan.BulbV2)
			d.AddSub(cid1, plug)
			d.AddSub(cid2, bulb)
			plug.Set("19", 615.0)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "client.py", "gateway", "host.docker.internal", port, v, id, localKey, cid1, cid2)
			st := event(t, p.expect(60*time.Second, "status"))
			if dps := dpsOf(st); dps["1"] != false || dps["19"] != 615.0 {
				t.Errorf("tinytuya sub-device status %v", st)
			}
			p.expect(10*time.Second, "turn_on")
			p.expect(10*time.Second, "updatedps")
			st2 := event(t, p.expect(10*time.Second, "status2"))
			if dps := dpsOf(st2); dps["1"] != true {
				t.Errorf("tinytuya status after turn_on %v", st2)
			}
			b := event(t, p.expect(10*time.Second, "bulb"))
			if dps := dpsOf(b); dps["21"] != "white" {
				t.Errorf("tinytuya bulb sub-device %v", b)
			}
			p.expect(10*time.Second, "done")
			if ctl := plug.Controls(); len(ctl) != 1 || ctl[0]["1"] != true {
				t.Errorf("plug controls %v", ctl)
			}
			if len(gw.Controls()) != 0 || len(bulb.Controls()) != 0 {
				t.Errorf("controls reached the gateway %v or the bulb %v", gw.Controls(), bulb.Controls())
			}
		})
	}
}

// tinytuya's BulbDevice detects the simulated bulb's layout and sets a
// colour and white light; the values it writes are the ones the tuyalan
// helpers write.
func TestTinytuyaBulbAgainstSimulator(t *testing.T) {
	needDocker(t)
	for _, c := range []struct {
		v string
		l tuyalan.BulbLayout
	}{{"3.3", tuyalan.BulbV1}, {"3.5", tuyalan.BulbV2}, {"3.4", tuyalan.BulbV2}} {
		t.Run(c.v+c.l.Name, func(t *testing.T) {
			mem := sim.NewBulb(c.l)
			id := "bfsimbulb" + strings.ReplaceAll(c.v, ".", "") + c.l.Name + "00001"
			_, port := startSim(t, c.v, id, mem)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "client.py", "bulb", "host.docker.internal", port, c.v, id, localKey)
			typ := event(t, p.expect(60*time.Second, "type"))
			if typ["type"] != c.l.Name {
				t.Errorf("tinytuya detected %v, want %s", typ, c.l.Name)
			}
			p.expect(10*time.Second, "set_colour")
			p.expect(10*time.Second, "set_white_percentage")
			p.expect(10*time.Second, "done")
			ctl := mem.Controls()
			if len(ctl) < 2 {
				t.Fatalf("controls %v", ctl)
			}
			colour, _ := c.l.SetRGB(10, 20, 200)
			white, _ := c.l.SetWhite(60, 30)
			for k, want := range colour {
				if got, ok := ctl[0][k]; ok && !same(got, want) {
					t.Errorf("colour %s: tinytuya wrote %v, tuyalan writes %v", k, got, want)
				}
			}
			if ctl[0][c.l.Colour] != colour[c.l.Colour] {
				t.Errorf("colour: tinytuya wrote %v, tuyalan writes %v", ctl[0], colour)
			}
			last := mem.Read(nil)
			for k, want := range white {
				if !same(last[k], want) {
					t.Errorf("white %s: bulb holds %v after tinytuya, tuyalan writes %v", k, last[k], want)
				}
			}
		})
	}
}

// same compares a decoded JSON value with a helper's value (numbers as
// float64 against int).
func same(got, want any) bool {
	if n, ok := want.(int); ok {
		f, ok := got.(float64)
		return ok && f == float64(n)
	}
	return got == want
}

// tinytuya's CoverDevice opens, positions and stops the simulated cover.
func TestTinytuyaCoverAgainstSimulator(t *testing.T) {
	needDocker(t)
	cv := sim.NewCover()
	id := "bfsimcover340001"
	_, port := startSim(t, "3.4", id, cv)
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "cover", "host.docker.internal", port, "3.4", id, localKey)
	st := event(t, p.expect(60*time.Second, "status"))
	if dps := dpsOf(st); dps["1"] != "stop" || dps["3"] != 40.0 {
		t.Errorf("tinytuya cover status %v", st)
	}
	p.expect(10*time.Second, "done")
	ctl := cv.Controls()
	if len(ctl) != 3 || ctl[0]["1"] != "open" || ctl[1]["2"] != 40.0 || ctl[2]["1"] != "stop" {
		t.Errorf("controls %v", ctl)
	}
	// The helpers send the same values.
	if m, _ := tuyalan.DefaultCover.Open(); m["1"] != ctl[0]["1"] {
		t.Errorf("Open %v", m)
	}
	if m, _ := tuyalan.DefaultCover.Stop(); m["1"] != ctl[2]["1"] {
		t.Errorf("Stop %v", m)
	}
}

// tinytuya's REQ_DEVINFO sender gets the simulator's announcement, which
// tinytuya decodes.
func TestTinytuyaDevInfoAgainstSimulator(t *testing.T) {
	needDocker(t)
	d, _ := startSim(t, "3.5", "bfsimdevinfo35001", sim.NewPlug())
	at, err := d.ServeDevInfo("0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(at)
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "devinfo", "host.docker.internal", port)
	ev := event(t, p.expect(60*time.Second, "devinfo"))
	js, _ := ev["json"].(map[string]any)
	if js["gwId"] != "bfsimdevinfo35001" || js["version"] != "3.5" || js["productKey"] != "keysim" {
		t.Errorf("tinytuya decoded %v", ev)
	}
	if d.DevInfoRequests() != 1 {
		t.Errorf("requests %d", d.DevInfoRequests())
	}
}

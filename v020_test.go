// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

func dial(t *testing.T, d *sim.Device) *tuyalan.Conn {
	t.Helper()
	c, err := tuyalan.Dial(context.Background(), d.Addr(), d.Config().Version, d.Config().ID, []byte(key), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func startWith(t *testing.T, version string, b sim.Backend) *sim.Device {
	t.Helper()
	d := sim.New(sim.Config{ID: "bfsim" + version + "dev", LocalKey: key, Version: version, ProductKey: "keysim"}, b)
	if _, err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	return d
}

// Discover's REQ_DEVINFO gets the simulator's announcement back.
func TestDiscoverAgainstSimulator(t *testing.T) {
	d := startWith(t, "3.5", sim.NewPlug())
	at, err := d.ServeDevInfo("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	heard, err := tuyalan.Discover(context.Background(), tuyalan.DiscoverOptions{
		Listen: "127.0.0.1", Ports: []int{}, Wait: time.Second, Interval: 200 * time.Millisecond, Request: []string{at},
	})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := heard[d.Config().ID]
	if !ok || h.Version != "3.5" || h.ProductKey != "keysim" || !h.Encrypted {
		t.Fatalf("heard %+v", heard)
	}
	if d.DevInfoRequests() < 2 {
		t.Errorf("requests %d", d.DevInfoRequests())
	}
	// Datagrams that are not requests are not answered. Requests Discover
	// sent just before it returned may still be arriving: let them land
	// before taking the count.
	time.Sleep(300 * time.Millisecond)
	n := d.DevInfoRequests()
	c, _ := net.Dial("udp", at)
	ann, _ := tuyalan.EncodeBroadcast(tuyalan.Broadcast{GwID: "x", Version: "3.5"})
	_, _ = c.Write(ann)
	c.Close()
	time.Sleep(100 * time.Millisecond)
	if d.DevInfoRequests() != n {
		t.Error("an announcement was counted as a request")
	}
}

// Scan finds the simulator's port; Identify tells its version with the
// local key.
func TestScanAndIdentify(t *testing.T) {
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			d := startWith(t, v, sim.NewPlug())
			found, err := tuyalan.Scan(context.Background(), []string{"127.0.0.1"}, tuyalan.ScanOptions{Port: d.Port(), Timeout: time.Second})
			if err != nil || len(found) != 1 || found[0] != d.Addr() {
				t.Fatalf("scan %v %v", found, err)
			}
			id, err := tuyalan.Identify(context.Background(), d.Addr(), d.Config().ID, []byte(key), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			want := tuyalan.Identity{Version: v}
			if v == "3.2" {
				want = tuyalan.Identity{Version: "3.3", Device22: true}
			}
			if id.Version != want.Version || id.Device22 != want.Device22 {
				t.Errorf("identity %+v, want %+v", id, want)
			}
			if !id.Device22 && id.DPS["20"] != 2301.0 {
				t.Errorf("data points %v", id.DPS)
			}
		})
	}
	// A wrong key identifies nothing (3.3, 3.4 and 3.5).
	for _, v := range []string{"3.3", "3.4", "3.5"} {
		d := startWith(t, v, sim.NewPlug())
		if id, err := tuyalan.Identify(context.Background(), d.Addr(), d.Config().ID, []byte("ffffffffffffffff"), 400*time.Millisecond); !errors.Is(err, tuyalan.ErrNotIdentified) {
			t.Errorf("%s with a wrong key: %+v %v", v, id, err)
		}
	}
}

// A gateway's sub-devices are read, set, refreshed and pushed by cid, in
// every version; the gateway's own data points stay apart.
func TestGatewaySubDevices(t *testing.T) {
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			gw := sim.NewMemory(map[string]any{"1": true})
			d := startWith(t, v, gw)
			plug, bulb := sim.NewPlug(), sim.NewBulb(tuyalan.BulbV2)
			d.AddSub("a4c138000000aa01", plug)
			d.AddSub("a4c138000000aa02", bulb)
			c := dial(t, d)
			s := c.Sub("a4c138000000aa01")
			if err := s.Query(); err != nil {
				t.Fatal(err)
			}
			m := recvUntil(t, c, func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			dps, _ := tuyalan.DPS(m.Payload)
			if tuyalan.CID(m.Payload) != "a4c138000000aa01" || dps["20"] != 2301.0 || dps["1"] != false {
				t.Fatalf("sub query %s", m.Payload)
			}
			cmd, seq, err := s.ControlSeq(map[string]any{"1": true})
			if err != nil {
				t.Fatal(err)
			}
			ack := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd })
			if ack.Seq != seq || ack.Ret != 0 {
				t.Errorf("ack %+v", ack)
			}
			push := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); tuyalan.CID(push.Payload) != "a4c138000000aa01" || dps["1"] != true {
				t.Errorf("sub push %s", push.Payload)
			}
			if plug.Read([]string{"1"})["1"] != true || gw.Read([]string{"1"})["1"] != true || len(gw.Controls()) != 0 {
				t.Errorf("the control reached the wrong device: plug %v gateway %v", plug.Read(nil), gw.Controls())
			}
			// UPDATE_DPS on the sub-device: acknowledged, then pushed.
			plug.Set("19", 615.0)
			if err := s.UpdateDPS([]int{19}); err != nil {
				t.Fatal(err)
			}
			recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdUpdateDPs && m.Ret == 0 })
			push = recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); tuyalan.CID(push.Payload) != "a4c138000000aa01" || dps["19"] != 615.0 || len(dps) != 1 {
				t.Errorf("update push %s", push.Payload)
			}
			// A change at the second sub-device.
			bulb.Set("22", 900.0)
			d.PushSub("a4c138000000aa02", map[string]any{"22": 900.0})
			push = recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if tuyalan.CID(push.Payload) != "a4c138000000aa02" {
				t.Errorf("PushSub %s", push.Payload)
			}
			// An unknown cid is refused.
			cmd, seq, _ = c.Sub("nosuchcid").ControlSeq(map[string]any{"1": true})
			ack = recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd })
			if ack.Seq != seq || ack.Ret == 0 {
				t.Errorf("unknown cid: %+v", ack)
			}
			// The gateway's own data points (3.2 is asked with null values).
			if v == "3.2" {
				err = c.QueryByControl([]string{"1"})
			} else {
				err = c.Query()
			}
			if err != nil {
				t.Fatal(err)
			}
			m = recvUntil(t, c, func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			if dps, _ := tuyalan.DPS(m.Payload); tuyalan.CID(m.Payload) != "" || len(dps) != 1 || dps["1"] != true {
				t.Errorf("gateway query %s", m.Payload)
			}
		})
	}
}

// UPDATE_DPS on a plain device.
func TestUpdateDPS(t *testing.T) {
	plug := sim.NewPlug()
	d := startWith(t, "3.4", plug)
	c := dial(t, d)
	plug.Set("18", 120.0)
	plug.Set("19", 250.0)
	if err := c.UpdateDPS([]int{18, 19, 77}); err != nil {
		t.Fatal(err)
	}
	push := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
	if dps, _ := tuyalan.DPS(push.Payload); len(dps) != 2 || dps["18"] != 120.0 || dps["19"] != 250.0 {
		t.Errorf("push %s", push.Payload)
	}
}

// The bulb helpers drive simulated bulbs of both layouts.
func TestBulbs(t *testing.T) {
	for _, l := range []tuyalan.BulbLayout{tuyalan.BulbV1, tuyalan.BulbV2, tuyalan.BulbDimmer} {
		t.Run(l.Name, func(t *testing.T) {
			mem := sim.NewBulb(l)
			d := startWith(t, "3.5", mem)
			c := dial(t, d)
			_ = c.Query()
			m := recvUntil(t, c, func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			dps, _ := tuyalan.DPS(m.Payload)
			got, ok := tuyalan.DetectBulb(dps)
			if !ok || got.Name != l.Name {
				t.Fatalf("detected %+v %v from %v", got, ok, dps)
			}
			set := func(dps map[string]any, err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
				cmd, _, err := c.ControlSeq(dps)
				if err != nil {
					t.Fatal(err)
				}
				recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd })
			}
			set(got.SetWhite(60, 30))
			st := got.State(mem.Read(nil))
			if !st.On || st.Mode != tuyalan.ModeWhite || st.BrightnessPct != 60 || st.ColourTempPct != 30 {
				t.Errorf("white: %+v", st)
			}
			if got.Colour == "" {
				return
			}
			set(got.SetRGB(10, 20, 200))
			st = got.State(mem.Read(nil))
			if st.Mode != tuyalan.ModeColour || st.Colour != tuyalan.ColourHexRGB(10, 20, 200, l.Format) {
				t.Errorf("colour: %+v", st)
			}
			if l.Name == "B" {
				set(got.SetCountdown(90))
				if mem.Read([]string{"26"})["26"] != 90.0 {
					t.Errorf("countdown %v", mem.Read(nil))
				}
			}
		})
	}
}

// The cover helpers drive the simulated cover.
func TestCover(t *testing.T) {
	cv := sim.NewCover()
	d := startWith(t, "3.3", cv)
	c := dial(t, d)
	_ = c.Query()
	m := recvUntil(t, c, func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
	dps, _ := tuyalan.DPS(m.Payload)
	cover := tuyalan.DefaultCover
	cover.Scheme = tuyalan.DetectCoverScheme(dps["1"])
	if cover.Scheme != tuyalan.CoverOpenCloseStop {
		t.Fatalf("scheme %d", cover.Scheme)
	}
	open, _ := cover.Open()
	_ = c.Control(open)
	push := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
	dps, _ = tuyalan.DPS(push.Payload)
	if p, ok := cover.PositionPct(dps); !ok || p != 100 || dps["1"] != "open" {
		t.Errorf("open: %v", dps)
	}
	pos, _ := cover.SetPositionPct(40)
	_ = c.Control(pos)
	push = recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
	dps, _ = tuyalan.DPS(push.Payload)
	if p, _ := cover.PositionPct(dps); p != 40 {
		t.Errorf("position: %v", dps)
	}
	cmd, _, _ := c.ControlSeq(map[string]any{"1": "sideways"})
	if ack := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd }); ack.Ret == 0 {
		t.Error("a bad cover value was accepted")
	}
	if len(cv.Controls()) != 2 || cv.Controls()[0]["1"] != "open" || len(cv.Controls()[0]) != 1 {
		t.Errorf("controls %v", cv.Controls())
	}
}

// A countdown set on the plug's data point 9. The guess CountdownDP makes
// (tinytuya's: the highest-numbered data point) is the voltage on a
// metering plug, so callers of such plugs name the data point.
func TestPlugCountdown(t *testing.T) {
	plug := sim.NewPlug()
	if dp := tuyalan.CountdownDP(plug.Read(nil)); dp != "20" {
		t.Errorf("guessed %s", dp)
	}
	d := startWith(t, "3.3", plug)
	c := dial(t, d)
	_ = c.Control(tuyalan.Countdown("9", 300))
	recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
	if plug.Read([]string{"9"})["9"] != 300.0 {
		t.Errorf("countdown %v", plug.Read(nil))
	}
}

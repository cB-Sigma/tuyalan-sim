// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

const key = "0123456789abcdef"

func startDevice(t *testing.T, version string) (*sim.Device, *sim.Memory) {
	t.Helper()
	mem := sim.NewMemory(map[string]any{"1": false, "2": 100.0})
	d := sim.New(sim.Config{ID: "bfsimplug" + strings.ReplaceAll(version, ".", ""), LocalKey: key, Version: version}, mem)
	if _, err := d.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	return d, mem
}

// recvUntil reads frames until one passes f.
func recvUntil(t *testing.T, c *tuyalan.Conn, f func(tuyalan.Message) bool) tuyalan.Message {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		m, err := c.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if f(m) {
			return m
		}
	}
}

// Every version: read, command (acknowledged with the request's sequence
// number, then pushed), heartbeat.
func TestSessionAllVersions(t *testing.T) {
	for _, v := range tuyalan.AllVersions {
		t.Run(v, func(t *testing.T) {
			d, mem := startDevice(t, v)
			c, err := tuyalan.Dial(context.Background(), d.Addr(), v, d.Config().ID, []byte(key), 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if v == "3.2" {
				// 3.2 devices refuse DP_QUERY: ask with null values.
				err = c.QueryByControl([]string{"1", "2"})
			} else {
				err = c.Query()
			}
			if err != nil {
				t.Fatal(err)
			}
			m := recvUntil(t, c, func(m tuyalan.Message) bool { _, ok := tuyalan.DPS(m.Payload); return ok })
			dps, _ := tuyalan.DPS(m.Payload)
			if dps["1"] != false || dps["2"] != 100.0 {
				t.Fatalf("dps = %v", dps)
			}
			cmd, seq, err := c.ControlSeq(map[string]any{"1": true})
			if err != nil {
				t.Fatal(err)
			}
			ack := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd })
			if ack.Seq != seq || ack.Ret != 0 {
				t.Errorf("ack seq %d ret %d, want seq %d ret 0", ack.Seq, ack.Ret, seq)
			}
			push := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdStatus })
			if dps, _ := tuyalan.DPS(push.Payload); dps["1"] != true {
				t.Errorf("push = %s", push.Payload)
			}
			if ctl := mem.Controls(); len(ctl) != 1 || ctl[0]["1"] != true {
				t.Errorf("controls = %v", ctl)
			}
			if err := c.Heartbeat(); err != nil {
				t.Fatal(err)
			}
			recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == tuyalan.CmdHeartBeat })
			if c.Version() != v || c.PlainAnswers() {
				t.Errorf("version %s, plain %v", c.Version(), c.PlainAnswers())
			}
		})
	}
}

// A wrong local key fails the negotiation of 3.4 and 3.5; refused and
// dropped controls are visible to the client.
func TestSessionFaults(t *testing.T) {
	for _, v := range []string{"3.4", "3.5"} {
		d, _ := startDevice(t, v)
		_, err := tuyalan.Dial(context.Background(), d.Addr(), v, d.Config().ID, []byte("ffffffffffffffff"), 2*time.Second)
		// The device cannot check the client's frame and drops the
		// session (or, when it could, its proof fails: ErrNegotiation).
		if err == nil {
			t.Errorf("%s with a wrong key negotiated", v)
		}
	}
	if _, err := tuyalan.Dial(context.Background(), "127.0.0.1:1", "3.6", "x", []byte(key), time.Second); err == nil {
		t.Error("version 3.6 accepted")
	}
	if _, err := tuyalan.Dial(context.Background(), "127.0.0.1:1", "3.3", "x", []byte("short"), time.Second); err == nil {
		t.Error("a short key accepted")
	}

	d, mem := startDevice(t, "3.3")
	c, err := tuyalan.Dial(context.Background(), d.Addr(), "3.3", d.Config().ID, []byte(key), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d.SetControlRet(1)
	cmd, seq, _ := c.ControlSeq(map[string]any{"1": true})
	ack := recvUntil(t, c, func(m tuyalan.Message) bool { return m.Cmd == cmd })
	if ack.Seq != seq || ack.Ret != 1 || len(mem.Controls()) != 0 {
		t.Errorf("refused control: seq %d ret %d controls %v", ack.Seq, ack.Ret, mem.Controls())
	}
	d.SetControlRet(0)
	d.SetNoControlAck(true)
	_ = c.Control(map[string]any{"1": true})
	_ = c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if m, err := c.Recv(); err == nil {
		t.Errorf("a dropped control was answered: %+v", m)
	}
}

// A device going off the network drops its sessions and refuses new ones.
func TestSessionDeviceDown(t *testing.T) {
	d, _ := startDevice(t, "3.5")
	c, err := tuyalan.Dial(context.Background(), d.Addr(), "3.5", d.Config().ID, []byte(key), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	d.SetDown(true)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Recv(); err == nil {
		t.Error("the session survived the device going down")
	}
	if _, err := tuyalan.Dial(context.Background(), d.Addr(), "3.5", d.Config().ID, []byte(key), time.Second); err == nil {
		t.Error("a down device negotiated")
	}
	d.SetDown(false)
	if _, err := tuyalan.Dial(context.Background(), d.Addr(), "3.5", d.Config().ID, []byte(key), 3*time.Second); err != nil {
		t.Error(err)
	}
	if d.Sessions() != 2 { // the refused one is not counted
		t.Errorf("sessions = %d", d.Sessions())
	}
}

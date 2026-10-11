// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

// refusing is a Backend that refuses every control as malformed.
type refusing struct{ *sim.Memory }

func (refusing) Write(map[string]any) (map[string]any, bool) { return nil, false }

func TestDevice(t *testing.T) {
	const key = "a1b2c3d4e5f6a7b8"
	mem := sim.NewMemory(map[string]any{"1": true})
	d := sim.New(sim.Config{ID: "bfsimtest0000001", LocalKey: key, Device22: true}, mem)
	if d.Config().Version != "3.3" || d.Addr() != "" || d.Port() != 0 || d.Sessions() != 0 {
		t.Fatal("defaults")
	}
	addr, err := d.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Stop()
	c, err := tuyalan.Dial(context.Background(), addr, "3.3", "bfsimtest0000001", []byte(key), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	// Device22: DP_QUERY is answered with "data unvalid".
	_ = c.Query()
	m, err := c.Recv()
	if err != nil || string(m.Payload) != "json obj data unvalid" {
		t.Fatalf("query = %q %v", m.Payload, err)
	}
	// A change made at the device reaches the session.
	mem.Set("1", false)
	d.Push(map[string]any{"1": false})
	m, err = c.Recv()
	if dps, _ := tuyalan.DPS(m.Payload); err != nil || m.Cmd != tuyalan.CmdStatus || dps["1"] != false {
		t.Fatalf("push = %+v %v", m, err)
	}
	// Unknown data points are ignored.
	if ch, ok := mem.Write(map[string]any{"99": 1}); !ok || len(ch) != 0 {
		t.Errorf("unknown DP: %v %v", ch, ok)
	}

	// A backend's refusal is answered with return code 1.
	r := sim.New(sim.Config{ID: "bfsimtest0000002", LocalKey: key}, refusing{sim.NewMemory(nil)})
	ra, _ := r.Start("127.0.0.1:0")
	defer r.Stop()
	rc, err := tuyalan.Dial(context.Background(), ra, "3.3", "bfsimtest0000002", []byte(key), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	_ = rc.SetReadDeadline(time.Now().Add(3 * time.Second))
	_ = rc.Control(map[string]any{"1": true})
	if m, err := rc.Recv(); err != nil || m.Ret != 1 {
		t.Errorf("refused control = %+v %v", m, err)
	}
}

func Example() {
	mem := sim.NewMemory(map[string]any{"1": false, "19": 0.0}) // switch, power
	d := sim.New(sim.Config{ID: "bfdemoplug0000001", LocalKey: "0123456789abcdef", Version: "3.4"}, mem)
	addr, _ := d.Start("127.0.0.1:0")
	defer d.Stop()

	c, _ := tuyalan.Dial(context.Background(), addr, "3.4", "bfdemoplug0000001", []byte("0123456789abcdef"), 3*time.Second)
	defer c.Close()
	_ = c.Control(map[string]any{"1": true})
	time.Sleep(100 * time.Millisecond)
	fmt.Println(mem.Read([]string{"1"}), len(mem.Controls()))
	// Output: map[1:true] 1
}

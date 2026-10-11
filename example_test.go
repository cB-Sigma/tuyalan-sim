// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"fmt"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

// Read a simulated 3.5 plug and switch it on.
func Example() {
	plug := sim.New(sim.Config{ID: "bfdemoplug0000001", LocalKey: "0123456789abcdef", Version: "3.5"},
		sim.NewMemory(map[string]any{"1": false, "19": 615.0}))
	addr, _ := plug.Start("127.0.0.1:0") // a real device listens on port 6668
	defer plug.Stop()

	c, err := tuyalan.Dial(context.Background(), addr, "3.5", "bfdemoplug0000001", []byte("0123456789abcdef"), 5*time.Second)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	_ = c.Query()
	for {
		m, err := c.Recv()
		if err != nil {
			fmt.Println(err)
			return
		}
		if dps, ok := tuyalan.DPS(m.Payload); ok {
			fmt.Println("switch:", dps["1"], "power:", dps["19"])
			break
		}
	}
	cmd, seq, _ := c.ControlSeq(map[string]any{"1": true})
	for {
		m, _ := c.Recv()
		if m.Cmd == cmd {
			fmt.Println("acknowledged:", m.Ret == 0, m.Seq == seq)
			break
		}
	}
	// Output:
	// switch: false power: 615
	// acknowledged: true true
}

// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

func TestRunner(t *testing.T) {
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := u.LocalAddr().(*net.UDPAddr).Port
	u.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	got := make(chan *sim.Device, 1)
	var out bytes.Buffer
	done := make(chan error, 1)
	args := []string{"-listen", "127.0.0.1:0", "-version", "3.5", "-broadcast", "127.0.0.1:" + strconv.Itoa(port), "-interval", "100ms"}
	heard := make(chan map[string]tuyalan.Heard, 1)
	go func() {
		h, _ := tuyalan.Hear(ctx, "127.0.0.1", []int{port}, time.Second)
		heard <- h
	}()
	time.Sleep(50 * time.Millisecond)
	go func() { done <- run(ctx, args, &out, func(d *sim.Device) { got <- d }) }()
	var d *sim.Device
	select {
	case d = <-got:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready")
	}
	if h := <-heard; h["bfdemoplug0000001"].Version != "3.5" {
		t.Errorf("heard %v", h)
	}
	c, err := tuyalan.Dial(ctx, d.Addr(), "3.5", "bfdemoplug0000001", []byte("0123456789abcdef"), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.Query()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	m, err := c.Recv()
	if dps, _ := tuyalan.DPS(m.Payload); err != nil || dps["20"] != 2301.0 {
		t.Errorf("query = %s %v", m.Payload, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "protocol 3.5") {
		t.Errorf("output = %s", out.String())
	}
}

func TestRunnerFlags(t *testing.T) {
	for _, args := range [][]string{{"-version", "2.9"}, {"-key", "short"}, {"-interval", "0s"}, {"-bogus"}} {
		if err := run(context.Background(), args, &bytes.Buffer{}, nil); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
}

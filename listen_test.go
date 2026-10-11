// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package tuyalan_test

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// Announcements of 3.1 (clear), 3.3 and 3.4 (ECB) and 3.5 (GCM) are heard,
// keyed by device ID, with the address the device announced.
func TestHear(t *testing.T) {
	p1, p2 := freeUDPPort(t), freeUDPPort(t)
	var devs []*sim.Device
	for _, v := range []string{"3.1", "3.3", "3.4", "3.5"} {
		d := sim.New(sim.Config{ID: "bfhear" + v, LocalKey: key, Version: v, ProductKey: "keysim"}, sim.NewMemory(nil))
		if _, err := d.Start("127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
		defer d.Stop()
		devs = append(devs, d)
	}
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for i := 0; ; i++ {
			for j, d := range devs {
				p := p1
				if j%2 == 1 {
					p = p2
				}
				_ = d.Broadcast(net.JoinHostPort("127.0.0.1", strconv.Itoa(p)))
			}
			select {
			case <-stop:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	heard, err := tuyalan.Hear(context.Background(), "127.0.0.1", []int{p1, p2}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range devs {
		h, ok := heard[d.Config().ID]
		if !ok || h.Version != d.Config().Version || h.Addr() != "127.0.0.1" || h.ProductKey != "keysim" || h.From == nil {
			t.Errorf("%s: %+v %v", d.Config().ID, h, ok)
		}
		if h.Encrypted != (d.Config().Version != "3.1") {
			t.Errorf("%s encrypted = %v", d.Config().Version, h.Encrypted)
		}
	}
	// A port that cannot be opened is reported; when none can, Hear fails.
	c, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer c.Close()
	busy := c.LocalAddr().(*net.UDPAddr).Port
	l, errs := tuyalan.ListenDiscovery("127.0.0.1", []int{busy, freeUDPPort(t)})
	if l == nil || len(errs) != 1 {
		t.Errorf("one busy port: %v %v", l, errs)
	} else {
		l.Close()
	}
	if _, err := tuyalan.Hear(context.Background(), "127.0.0.1", []int{busy}, 100*time.Millisecond); err == nil {
		t.Error("Hear on a busy port succeeded")
	}
	if (tuyalan.Heard{From: net.IPv4(10, 0, 0, 9)}).Addr() != "10.0.0.9" {
		t.Error("Addr without an announced IP")
	}
}

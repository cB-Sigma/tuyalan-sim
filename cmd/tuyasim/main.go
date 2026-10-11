// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Command tuyasim serves a simulated Tuya device on the LAN protocol, so
// a Tuya LAN client can be tried without a device, speaking protocol
// -version under the local key -key. -type picks the device: "plug" (the
// default; a metering plug with data points 1 switch, 9 countdown, 18
// current in mA, 19 power in 0.1 W and 20 voltage in 0.1 V), "bulb" (the
// newer bulb layout, from data point 20), "bulbv1" (the older one, from
// 1), "dimmer", "cover" (1 control, 2 and 3 position), or "gateway" (a
// gateway with a plug sub-device a4c1380000000001 and a bulb
// a4c1380000000002). With -broadcast it announces itself every -interval
// as a device of its version does (send to port 6666 for 3.1, 6667 for 3.2
// to 3.4, 7000 for 3.5); with -devinfo it answers REQ_DEVINFO requests on
// that UDP address. For example:
//
//	tuyasim -listen 127.0.0.1:6668 -version 3.4 -broadcast 127.0.0.1:6667
//	tuyasim -type bulb -version 3.5 -devinfo 127.0.0.1:7000
//
// The device ID and local key are demo values.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cB-Sigma/tuyalan"
	"github.com/cB-Sigma/tuyalan-sim/sim"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, nil); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "tuyasim:", err)
		os.Exit(1)
	}
}

// run serves until ctx ends. ready, when not nil, receives the device once
// it listens.
func run(ctx context.Context, args []string, out io.Writer, ready func(*sim.Device)) error {
	fs := flag.NewFlagSet("tuyasim", flag.ContinueOnError)
	fs.SetOutput(out)
	listen := fs.String("listen", "127.0.0.1:6668", "TCP address of the device")
	version := fs.String("version", "3.3", "protocol version: 3.1, 3.2, 3.3, 3.4 or 3.5")
	id := fs.String("id", "bfdemoplug0000001", "device ID")
	key := fs.String("key", "0123456789abcdef", "local key (16 characters)")
	broadcast := fs.String("broadcast", "", "send the discovery announcement to this host:port")
	interval := fs.Duration("interval", 5*time.Second, "announcement interval")
	kind := fs.String("type", "plug", "device: plug, bulb, bulbv1, dimmer, cover or gateway")
	devinfo := fs.String("devinfo", "", "answer REQ_DEVINFO requests on this UDP host:port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !tuyalan.ValidVersion(*version) {
		return fmt.Errorf("-version %q is not 3.1 to 3.5", *version)
	}
	if len(*key) != 16 {
		return errors.New("-key must be 16 characters")
	}
	if *interval <= 0 {
		return errors.New("-interval must be positive")
	}
	var b sim.Backend
	switch *kind {
	case "plug":
		b = sim.NewPlug()
	case "gateway":
		b = sim.NewMemory(map[string]any{})
	case "bulb":
		b = sim.NewBulb(tuyalan.BulbV2)
	case "bulbv1":
		b = sim.NewBulb(tuyalan.BulbV1)
	case "dimmer":
		b = sim.NewBulb(tuyalan.BulbDimmer)
	case "cover":
		b = sim.NewCover()
	default:
		return fmt.Errorf("-type %q is not plug, bulb, bulbv1, dimmer, cover or gateway", *kind)
	}
	d := sim.New(sim.Config{ID: *id, LocalKey: *key, Version: *version, ProductKey: "keydemo" + *kind}, b)
	if *kind == "gateway" {
		d.AddSub("a4c1380000000001", sim.NewPlug())
		d.AddSub("a4c1380000000002", sim.NewBulb(tuyalan.BulbV2))
	}
	addr, err := d.Start(*listen)
	if err != nil {
		return err
	}
	defer d.Stop()
	fmt.Fprintf(out, "tuyasim: %s %s (protocol %s) on tcp %s\n", *kind, *id, *version, addr)
	if *devinfo != "" {
		at, err := d.ServeDevInfo(*devinfo)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "tuyasim: answering REQ_DEVINFO on udp %s\n", at)
	}
	if ready != nil {
		ready(d)
	}
	var tick <-chan time.Time
	if *broadcast != "" {
		if err := d.Broadcast(*broadcast); err != nil {
			return err
		}
		t := time.NewTicker(*interval)
		defer t.Stop()
		tick = t.C
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick:
			_ = d.Broadcast(*broadcast)
		}
	}
}

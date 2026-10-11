// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Command tuyasim serves a simulated Tuya plug on the LAN protocol, so a
// Tuya LAN client can be tried without a device: a metering plug with
// data points 1 (switch), 18 (current, mA), 19 (power, 0.1 W) and 20
// (voltage, 0.1 V), speaking protocol -version under the local key -key.
// With -broadcast it announces itself every -interval as a device of its
// version does (send to port 6666 for 3.1, 6667 for 3.2 to 3.4, 7000 for
// 3.5). For example:
//
//	tuyasim -listen 127.0.0.1:6668 -version 3.4 -broadcast 127.0.0.1:6667
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
	mem := sim.NewMemory(map[string]any{"1": false, "18": 0.0, "19": 0.0, "20": 2301.0})
	d := sim.New(sim.Config{ID: *id, LocalKey: *key, Version: *version, ProductKey: "keydemoplug"}, mem)
	addr, err := d.Start(*listen)
	if err != nil {
		return err
	}
	defer d.Stop()
	fmt.Fprintf(out, "tuyasim: plug %s (protocol %s) on tcp %s\n", *id, *version, addr)
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

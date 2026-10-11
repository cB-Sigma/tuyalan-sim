// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/tuyalan-sim/sim"
)

// tinytuya's subdev_query reaches the simulated gateway (3.4, 3.5) and
// decodes its answer: one sub-device online, one offline.
func TestTinytuyaSubdevQueryAgainstSimulator(t *testing.T) {
	needDocker(t)
	const cid1, cid2 = "a4c1380000000001", "a4c1380000000002"
	for _, v := range []string{"3.4", "3.5"} {
		t.Run(v, func(t *testing.T) {
			id := "bfsimsubq" + strings.ReplaceAll(v, ".", "") + "00001"
			d, port := startSim(t, v, id, sim.NewMemory(map[string]any{"1": true}))
			d.AddSub(cid1, sim.NewPlug())
			d.AddSub(cid2, sim.NewPlug())
			d.SetSubOffline(cid2, true)
			p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
				"python", "-u", "client.py", "subdev", "host.docker.internal", port, v, id, localKey)
			ev := event(t, p.expect(60*time.Second, "subdev"))
			data, _ := ev["data"].(map[string]any)
			if fmt.Sprint(data["online"]) != "["+cid1+"]" || fmt.Sprint(data["offline"]) != "["+cid2+"]" {
				t.Errorf("tinytuya decoded %v", ev)
			}
			p.expect(10*time.Second, "done")
		})
	}
}

// tinytuya's detect_available_dps finds the data points of a device22
// simulator (it switches to device22 on the "data unvalid" answer). On the
// first run tinytuya 1.20.0 resets its request list to data point 1 when it
// switches, so the range that triggered the switch (2 to 10, here data
// point 9) is not asked; a second run, already device22, finds all of
// them. tuyalan's DetectAvailableDPS asks every range.
func TestTinytuyaDetectAgainstSimulator(t *testing.T) {
	needDocker(t)
	id := "bfsimdetect33001"
	d := sim.New(sim.Config{ID: id, LocalKey: localKey, Version: "3.3", Device22: true}, sim.NewPlug())
	addr, err := d.Start("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Stop)
	port := addr[strings.LastIndex(addr, ":")+1:]
	p := startPeer(t, "--add-host=host.docker.internal:host-gateway", image,
		"python", "-u", "client.py", "detect", "host.docker.internal", port, "3.3", id, localKey)
	ev := event(t, p.expect(90*time.Second, "detect"))
	list := func(k string) []string {
		var out []string
		for _, x := range ev[k].([]any) {
			out = append(out, x.(string))
		}
		return out
	}
	if got := list("dps"); !reflect.DeepEqual(got, []string{"1", "18", "19", "20"}) {
		t.Errorf("tinytuya's first run %v", got)
	}
	if got := list("again"); !reflect.DeepEqual(got, []string{"1", "9", "18", "19", "20"}) || ev["dev_type"] != "device22" {
		t.Errorf("tinytuya's second run %v", ev)
	}
	p.expect(10*time.Second, "done")
}

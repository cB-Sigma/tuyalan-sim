# tuyalan-sim

A simulated Tuya Wi-Fi device on the local LAN protocol, versions 3.1 to 3.5, for testing Tuya LAN clients without a device; not affiliated with Tuya.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/tuyalan-sim.svg)](https://pkg.go.dev/github.com/cB-Sigma/tuyalan-sim) [![test](https://github.com/cB-Sigma/tuyalan-sim/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/tuyalan-sim/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.22-00ADD8.svg)](go.mod)

Each simulated device is a TCP server speaking one protocol version:
the 3.4 and 3.5 session key negotiation, DP_QUERY and DP_QUERY_NEW,
CONTROL and CONTROL_NEW acknowledged with the request's sequence number
and followed by status pushes, heartbeats, and the UDP discovery
announcement of its version. Its data points live behind a `Backend`, so
a test can keep them wherever its other fakes do. It is built on
[tuyalan](https://github.com/cB-Sigma/tuyalan). Written for SigmaM2M, a
oneM2M platform; it has no dependency on it.

## What is simulated

| Feature | Status |
|---|---|
| Protocol 3.1 to 3.5, one version per device | yes |
| Session key negotiation (3.4, 3.5); a wrong local key ends the session | yes |
| DP_QUERY / DP_QUERY_NEW; "data unvalid" for DP_QUERY on 3.2 and on `Device22` devices; null-value CONTROL_NEW queries | yes |
| CONTROL / CONTROL_NEW acknowledged with the request's sequence number and an empty payload, then a status push to every session | yes |
| Pushes started at the device (`Device.Push`), heartbeats | yes |
| Discovery announcement of the device's version (`Device.Broadcast`) | yes |
| Faults: refused controls (`SetControlRet`), dropped controls (`SetNoControlAck`), the device off the network (`SetDown`), stop and restart on the same address | yes |
| Gateways and sub-devices: requests with a cid go to that sub-device's `Backend` (`AddSub`), answers and pushes carry the cid (`PushSub`); an unknown cid is refused | yes |
| UPDATE_DPS: acknowledged, then the asked data points pushed | yes |
| A gateway's sub-device online list: the LAN_EXT_STREAM `subdev_online_stat_query` answered with the online and offline cids (`SetSubOffline`); 3.3 gateways stay silent, as real ones are reported to | yes (one answer, without "reqType"; a real gateway's exact answer has not been captured) |
| REQ_DEVINFO answers: the announcement sent back to the request's source (`ServeDevInfo`, `DevInfoRequests`) | yes (in any version; real devices that answer are 3.5) |
| Device types: metering plug (`NewPlug`), bulbs of both layouts and a white-only bulb (`NewBulb`), a curtain motor that reaches positions at once (`NewCover`) | yes |
| More device types on the tuyalan `contrib` layouts: floor fan (`NewFan`), air conditioner (`NewClimate`), 24 V thermostat with a remote sensor list (`NewThermostat`), IR blasters of control type 1 and 2 that record commands, follow study mode and report learned codes (`NewIRBlaster`, `IRBlaster.Learn`, `Commands`, `Studying`) | yes (the IR blaster sends no infrared; a learned code is one the test supplies) |
| A real device's timing, a cover's travel time, colour effects | no |

## Install

```
go get github.com/cB-Sigma/tuyalan-sim@latest
```

Requires Go 1.22 or later. The command:

```
go install github.com/cB-Sigma/tuyalan-sim/cmd/tuyasim@latest
```

## Command line

```
tuyasim -listen 127.0.0.1:6668 -version 3.4 -broadcast 127.0.0.1:6667
```

| Flag | Default | Meaning |
|---|---|---|
| `-listen` | `127.0.0.1:6668` | TCP address of the device |
| `-version` | `3.3` | protocol version: 3.1, 3.2, 3.3, 3.4 or 3.5 |
| `-id` | `bfdemoplug0000001` | device ID |
| `-key` | `0123456789abcdef` | local key (16 characters) |
| `-broadcast` | | send the discovery announcement to this host:port (6666 for 3.1, 6667 for 3.2 to 3.4, 7000 for 3.5) |
| `-interval` | `5s` | announcement interval |
| `-type` | `plug` | `plug`, `bulb` (layout from data point 20), `bulbv1` (from 1), `dimmer`, `cover` or `gateway` |
| `-devinfo` | | answer REQ_DEVINFO requests on this UDP host:port (devices use 7000) |

| Type | Data points |
|---|---|
| `plug` | 1 switch, 9 countdown (s), 18 current (mA), 19 power (0.1 W), 20 voltage (0.1 V) |
| `bulb` | 20 switch, 21 mode, 22 brightness and 23 colour temperature (10-1000), 24 colour (HSV16), 25 scene, 26 countdown |
| `bulbv1` | 1 switch, 2 mode, 3 brightness and 4 colour temperature (25-255), 5 colour (RGB8) |
| `dimmer` | 1 switch, 2 brightness, 3 colour temperature (25-255) |
| `cover` | 1 control (open, close, stop, continue), 2 position to reach, 3 position reached (%) |
| `gateway` | none of its own; a `plug` sub-device with cid a4c1380000000001 and a `bulb` with a4c1380000000002, both online for the sub-device online query (3.4 and 3.5) |
| `fan` | 1 power, 2 mode, 3 speed (1-5), 5 oscillation, 22 timer |
| `climate` | 1 power, 2 target and 3 room temperature, 4 mode, 5 fan speed, 19 unit, 22 timer, 25 sleep, 30 swing, 101 state |
| `thermostat` | the low-resolution layout (2 mode, 16/17 setpoint, 18-20 and 26 cooling and heating setpoints, 24/29 temperature, 115 fan, 120 hold, 129 system and others) and a sensor list on 122 |
| `irblaster`, `irblaster2` | IR blaster of control type 1 (201 commands, 202 learned code) or 2 (1 mode, 2 learned, 3 head, 4 key, 7 code, 13 code type) |

## In Go tests

From `Example` in example_test.go:

```go
plug := sim.New(sim.Config{ID: "bfdemoplug0000001", LocalKey: "0123456789abcdef", Version: "3.5"},
	sim.NewMemory(map[string]any{"1": false, "19": 615.0}))
addr, _ := plug.Start("127.0.0.1:0") // a real device listens on port 6668
defer plug.Stop()

c, err := tuyalan.Dial(ctx, addr, "3.5", "bfdemoplug0000001", []byte("0123456789abcdef"), 5*time.Second)
_ = c.Query()
// ... c.Recv() returns {"1": false, "19": 615}
cmd, seq, _ := c.ControlSeq(map[string]any{"1": true})
// ... the acknowledgement carries cmd and seq, then a push of {"1": true}
```

## Documentation

Reference: https://pkg.go.dev/github.com/cB-Sigma/tuyalan-sim

| Package | What it does |
|---|---|
| `sim` | `New(Config, Backend)`, `Device` (`Start`, `Stop`, `Push`, `Broadcast`, `ServeDevInfo`, `DevInfoRequests`, `AddSub`, `PushSub`, `SetSubOffline`, `SetControlRet`, `SetNoControlAck`, `SetDown`, `Sessions`), `Memory` (a ready `Backend` that records controls), `NewPlug`, `NewBulb`, `NewCover` / `Cover`, `NewFan`, `NewClimate`, `NewThermostat`, `NewIRBlaster` / `IRBlaster` |
| `cmd/tuyasim` | Serves a plug, bulb, dimmer, cover, gateway, fan, air conditioner, thermostat or IR blaster of any version, announces it and answers REQ_DEVINFO |
| `interop` (nested module, tests only) | tinytuya's client, child devices, bulb, cover, IR remote, floor fan and thermostat classes, sub-device online query, data point detection, REQ_DEVINFO sender and announcement decoder against the simulator, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TUYALAN_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the tuyalan client against the
simulator: every version, faults, a device going down, discovery
(listening and REQ_DEVINFO), scanning, `Identify` and
`IdentifyWithKeys`, gateways and sub-devices with their online list,
UPDATE_DPS, `DetectAvailableDPS`, the persistent `Device` (status, set,
pushes, cache, reconnection after the network drops, for 3.3, 3.4 and
3.5; its status answer is never a change push still on its way, and a
device22 device is read with `QueryDPS`), the bulb, cover and countdown
helpers, and the tuyalan `contrib` helpers driving the fan, air
conditioner, thermostat (state, sensor list, setpoint, a renamed
sensor) and both kinds of IR blaster (study, a learned code pushed,
sending it). Tests carry
`// Covers: SIG-...` comments: requirement IDs of the SigmaM2M project's
compliance report, with no meaning outside it; the tests added in v0.2.0
and v0.3.0 carry none yet.

### Interop tested against

- tinytuya 1.20.0's client (2026-10-11, python:3.12.10-slim in Docker
  Desktop on arm64, rerun on that image for v0.2.0 after v0.1.0 had
  run on python:3.14-slim; `interop`): for 3.1, 3.2, 3.3, 3.4 and 3.5, its
  `OutletDevice` read the data points (3.2 as a device22 device),
  switched on, set a value, sent a heartbeat, read the changes back and
  received a push started at the simulator; the simulator recorded the
  two controls. tinytuya's `decrypt_udp` read the simulator's
  announcement of every version on the port of that version (6666, 6667,
  7000). The first run found that acknowledgements carried an encrypted
  empty payload, which tinytuya took for invalid JSON; fixed in v0.1.0.
- Same image, v0.2.0 device types, all passed: for every version,
  tinytuya's child devices (an `OutletDevice` and a `BulbDevice` with
  `cid` and `parent` set) read a plug sub-device of the simulated
  gateway, switched it on, sent UPDATE_DPS and read it back, and read
  the bulb sub-device; only the plug's backend received the control.
  (The 3.2 run takes about 45 s: tinytuya probes a 3.2 gateway for its
  data points first.) tinytuya's `BulbDevice` detected the older layout
  on a 3.3 simulator and the newer one on 3.4 and 3.5, and its
  `set_colour(10, 20, 200)` and `set_white_percentage(60, 30)` wrote the
  same values the tuyalan helpers write. tinytuya's `CoverDevice`
  opened the 3.4 cover, set position 40 and stopped it. tinytuya's own
  REQ_DEVINFO sender (`scanner.send_discovery_request`) got the 3.5
  simulator's announcement back through Docker's UDP port mapping and
  decoded it.
- Same image, v0.3.0 (2026-10-12, Docker Engine 29.4.3 on arm64; the
  whole suite was rerun and passed): tinytuya's `subdev_query` reached
  the simulated 3.4 and 3.5 gateways and returned their answer with one
  sub-device online and one offline. tinytuya's `detect_available_dps`
  against a 3.3 device22 simulator found data points 1, 18, 19 and 20 on
  its first run and 1, 9, 18, 19 and 20 on a second: tinytuya resets its
  request list to data point 1 when it switches a device to device22, so
  the range that triggered the switch is not asked (tinytuya's own
  behaviour, read in its debug log; tuyalan's `DetectAvailableDPS` asks
  every range).
- Same image, later on 2026-10-12, the new device types (whole suite
  rerun, all passed): tinytuya's `IRRemoteControlDevice`, given no
  control type, detected type 1 and type 2 on the simulated blasters,
  sent a learned code and a head/key that the simulator decoded into the
  commands tuyalan's `contrib` helpers build, and its `receive_button`
  got the code the simulator pushed during the study. tinytuya's
  `FloorFanDevice` switched, set mode, speed, oscillation and timer on
  the 3.4 fan; the five controls are the `contrib` helpers' controls.
  tinytuya's `ThermostatDevice` read the 3.3 thermostat's resolution,
  setpoint, temperature and remote sensor (ID, name, temperature,
  battery) as `contrib` reads them, and its cooling setpoint and sensor
  rename are the controls `contrib` builds.
- Not yet: compared with a real Tuya device (needs hardware). The simulator and the
  tuyalan client are written by the same author, so their agreement is
  not independent; the tinytuya runs are.

## Demo values

Every device ID, local key, product key and address in this module is a
made-up demo value. Do not use them for real devices.

## Related repositories

- [tuyalan](https://github.com/cB-Sigma/tuyalan): the Tuya LAN protocol
  client this module is built on.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam. Tuya is a trademark of its owner; the name is used only
to say which devices this module simulates.

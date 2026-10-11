# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.3.0 (2026-10-12)

Additions only; needs tuyalan v0.3.0.

- A gateway answers the sub-device online query (LAN_EXT_STREAM,
  `subdev_online_stat_query`) with its online and offline cids, in 3.4
  and 3.5; `Device.SetSubOffline` marks a sub-device offline. The
  `tuyasim` gateway type answers it too.
- Device types on the tuyalan `contrib` layouts: `NewFan`, `NewClimate`,
  `NewThermostat` (with a remote sensor list) and `NewIRBlaster` /
  `IRBlaster` (control type 1 or 2; records commands, follows study
  mode, `Learn` reports a learned code). `tuyasim -type` gains fan,
  climate, thermostat, irblaster and irblaster2.
- Tests driving tuyalan v0.3.0 against the simulator: `IdentifyWithKeys`,
  `SubdevQuery`, `DetectAvailableDPS`, the persistent `Device`
  (including reconnection after `SetDown`, device22 reads with
  `QueryDPS`, and a status right after a set), and the `contrib`
  helpers on the new device types.
- Interop: tinytuya's `subdev_query`, `detect_available_dps`,
  `IRRemoteControlDevice` (type detection, sending, learning),
  `FloorFanDevice` and `ThermostatDevice` against the simulator.
- Test fix: `TestDiscoverAgainstSimulator` took its request count while
  `Discover`'s last requests could still arrive, and failed now and then;
  it now waits for them first.

## v0.2.0 (2026-10-11)

Additions only; needs tuyalan v0.2.0.

- Gateways: `Device.AddSub` gives a device sub-devices addressed by cid,
  each with its own `Backend`; answers and pushes about them carry the
  cid; `Device.PushSub`; an unknown cid is refused with return code 1.
- UPDATE_DPS is acknowledged and the asked data points are pushed.
- `Device.ServeDevInfo` answers REQ_DEVINFO with the announcement, sent
  to the request's source; `Device.DevInfoRequests` counts requests.
- Device types: `NewPlug` (metering plug with countdown 9), `NewBulb`
  (either bulb layout or a white-only bulb), `NewCover` / `Cover` (a
  curtain motor on data points 1, 2 and 3).
- `tuyasim` gains `-type` (plug, bulb, bulbv1, dimmer, cover, gateway)
  and `-devinfo`; the plug also has data point 9.
- Interop: the image is python:3.12.10-slim (was python:3.14-slim) and
  is rebuilt (from cache) on every run; new tests run tinytuya's child
  devices, `BulbDevice`, `CoverDevice` and REQ_DEVINFO sender against
  the simulator.

## v0.1.0 (2026-10-11)

First release as a module of its own, split from the tuyalan module: a
simulated device of protocol 3.1 to 3.5 (`sim.New`, `Device`, `Backend`,
`Memory`, fault switches, announcements) and `cmd/tuyasim`.

- Fixed: a control's acknowledgement and a heartbeat answer have an
  empty payload in every version, as devices send them; tinytuya
  reported the former encrypted empty payload as invalid JSON.
- The tuyalan client's session and discovery tests against the
  simulator moved here from tuyalan.
- Interop tests running tinytuya 1.20.0's client against the simulator,
  in a nested test-only module, `interop`.

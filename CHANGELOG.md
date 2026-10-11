# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

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

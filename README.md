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
| Gateways and sub-devices, REQ_DEVINFO answers, a real device's timing | no |

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

The device is a metering plug: data points 1 (switch), 18 (current, mA),
19 (power, 0.1 W) and 20 (voltage, 0.1 V).

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
| `sim` | `New(Config, Backend)`, `Device` (`Start`, `Stop`, `Push`, `Broadcast`, `SetControlRet`, `SetNoControlAck`, `SetDown`, `Sessions`), `Memory` (a ready `Backend` that records controls) |
| `cmd/tuyasim` | Serves a metering plug of any version and announces it |
| `interop` (nested module, tests only) | tinytuya's client and announcement decoder against the simulator, in a container |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
cd interop && SIGMA_TUYALAN_INTEROP=1 go test -v ./...   # needs Docker
```

The tests at the module root drive the tuyalan client against the
simulator: every version, faults, a device going down, discovery.
Tests carry `// Covers: SIG-...` comments: requirement IDs of the
SigmaM2M project's compliance report, with no meaning outside it.

### Interop tested against

- tinytuya 1.20.0's client (2026-10-11, python:3.14-slim in Docker
  Desktop on arm64; `interop`): for 3.1, 3.2, 3.3, 3.4 and 3.5, its
  `OutletDevice` read the data points (3.2 as a device22 device),
  switched on, set a value, sent a heartbeat, read the changes back and
  received a push started at the simulator; the simulator recorded the
  two controls. tinytuya's `decrypt_udp` read the simulator's
  announcement of every version on the port of that version (6666, 6667,
  7000). The first run found that acknowledgements carried an encrypted
  empty payload, which tinytuya took for invalid JSON; fixed in v0.1.0.
- Not yet: compared with a real Tuya device. The simulator and the
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

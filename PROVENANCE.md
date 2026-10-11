# Provenance

## How the code was written

Written for SigmaM2M, a oneM2M platform, in Go, by the author of the
tuyalan module and from the same public descriptions of the protocol
(chiefly tinytuya, MIT; facts only). No code was ported or copied from
another implementation. The simulator was first written next to
SigmaM2M's Tuya adapter to test it without devices; the `Backend`
interface was added when it became part of the tuyalan module
(2026-10-06). The simulator, its command and the session and discovery
tests that drive it moved out of tuyalan into this module on 2026-10-11
(with `git mv`), so the library does not depend on its simulator.

On 2026-10-11 tinytuya 1.20.0's client was run against the simulator
(`interop`). It found one simulator bug, fixed the same day: a control's
acknowledgement and a heartbeat answer carried an encrypted empty
payload (or a version header alone), which tinytuya reported as invalid
JSON; devices send an empty payload, and the simulator now does
(`TestEmptyAnswersHaveEmptyPayloads`).

v0.2.0 (2026-10-11) added gateways with sub-devices, UPDATE_DPS,
REQ_DEVINFO answers and the plug, bulb and cover device types
(sim/types.go), after reading tinytuya 1.20.0 for behaviour only:
where a gateway's answers carry the cid (core/XenonDevice.py), what a
REQ_DEVINFO request looks like (scanner.py), the bulb layouts and the
cover values its classes expect (BulbDevice.py, CoverDevice.py). How
the simulator answers (UPDATE_DPS acknowledged then pushed, unknown
cids refused, the announcement sent back to the request's source, the
cover reaching positions at once) is our own choice where tinytuya does
not show a device's side. The interop image moved to
python:3.12.10-slim and was rerun on it.

v0.3.0 (2026-10-11/12) added the answer to a gateway's sub-device
online query (LAN_EXT_STREAM) and `SetSubOffline`. The request comes
from tinytuya 1.20.0 (core/XenonDevice.py, subdev_query); the answer's
fields ("online" and "offline" in "data") and that 3.3 gateways do not
answer were read from hass-localtuya (GPL-3.0; facts only, no code).
Whether a real gateway adds a "reqType" to its answer, or splits long
lists across answers, is not known; the simulator sends one answer
without it. tinytuya's subdev_query and detect_available_dps were run
against the simulator.

Later the same release (2026-10-12) added the fan, air conditioner,
thermostat and IR blaster types (sim/contrib.go), on the data point
layouts of the tuyalan contrib package, which come from tinytuya
1.20.0's Contrib classes (facts only). How the IR blaster behaves (it
records commands, enters and leaves study mode, and reports a learned
code only when a test supplies one) is our own choice; tinytuya does not
show a blaster's side beyond its data points. tinytuya's
IRRemoteControlDevice, FloorFanDevice and ThermostatDevice were run
against these types.

## Sources

| Area (files) | Followed |
|---|---|
| Simulator (sim/sim.go) | the tuyalan module's codec and frames; device behaviour (acknowledgements with the request's sequence number, pushes after changes, "data unvalid" for DP_QUERY on 3.2 and device22 devices, null-value queries) from tinytuya's descriptions |
| Command (cmd/tuyasim) | none (flags are our own) |
| Device types (sim/types.go) | data point layouts from tinytuya 1.20.0 (MIT) BulbDevice.py and CoverDevice.py, through the tuyalan helpers; the plug keeps the data points of the v0.1.0 command and adds countdown 9 |
| Contrib device types (sim/contrib.go) | the tuyalan contrib package's layouts (from tinytuya 1.20.0's Contrib classes, MIT); the IR blaster's behaviour is our own |
| Sub-device online answer (sim/sim.go) | the request from tinytuya 1.20.0 (MIT); the answer's field names from hass-localtuya (GPL-3.0), facts only |
| Interop peer (interop/peer/client.py) | our own script calling tinytuya 1.20.0 (OutletDevice, BulbDevice, CoverDevice, child devices, subdev_query, detect_available_dps, the Contrib IRRemoteControlDevice, FloorFanDevice and ThermostatDevice, scanner.send_discovery_request, decrypt_udp), installed from PyPI at test time into python:3.12.10-slim |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| session_test.go, listen_test.go, v020_test.go, v030_test.go, contrib_test.go, example_test.go, sim/, cmd/tuyasim | the tuyalan client against this simulator, written from the same descriptions; not a real device |
| interop/ | tinytuya 1.20.0's client, device classes, REQ_DEVINFO sender and announcement decoder against this simulator |

All device IDs, local keys and addresses are made-up demo values.

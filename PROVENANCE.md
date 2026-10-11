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

## Sources

| Area (files) | Followed |
|---|---|
| Simulator (sim/sim.go) | the tuyalan module's codec and frames; device behaviour (acknowledgements with the request's sequence number, pushes after changes, "data unvalid" for DP_QUERY on 3.2 and device22 devices, null-value queries) from tinytuya's descriptions |
| Command (cmd/tuyasim) | none (flags are our own) |
| Interop peer (interop/peer/client.py) | our own script calling tinytuya 1.20.0, installed from PyPI at test time |

No specification or vendor text is reproduced.

## Tests

| Test | Source |
|---|---|
| session_test.go, listen_test.go, example_test.go, sim/, cmd/tuyasim | the tuyalan client against this simulator, written from the same descriptions; not a real device |
| interop/ | tinytuya 1.20.0's client and announcement decoder against this simulator |

All device IDs, local keys and addresses are made-up demo values.

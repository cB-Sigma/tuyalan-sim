# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# tinytuya's client against a simulated device, for interop tests.
#
#   python client.py session <host> <port> <version> <device id> <local key>
#   python client.py listen
#
# "session" reads, switches and sets a device with tinytuya's
# OutletDevice and prints "PEER <step> <json>" after each step; after
# "PEER waiting" it reads one status push. "listen" binds UDP 6666, 6667
# and 7000 and prints each announcement as tinytuya decodes it.

import json
import socket
import sys
import select

import tinytuya
from tinytuya.core.udp_helper import decrypt_udp


def say(step, obj=None):
    print("PEER " + step + ("" if obj is None else " " + json.dumps(obj, sort_keys=True, default=str)), flush=True)


def session(host, port, version, dev_id, key):
    d = tinytuya.OutletDevice(dev_id, host, key, version=float(version), port=int(port), connection_timeout=5, connection_retry_limit=2, connection_retry_delay=1)
    d.set_socketPersistent(True)
    if version == "3.2":
        # device22: data points are asked for by name
        d.set_dpsUsed({"1": None, "2": None})
    say("status", d.status())
    say("turn_on", d.turn_on())
    say("set_value", d.set_value(2, 123))
    say("heartbeat", d.heartbeat())
    say("status2", d.status())
    say("waiting")
    for _ in range(5):
        r = d.receive()
        if r and "dps" in r:
            say("push", r)
            break
    d.close()
    say("done")


def listen():
    socks = []
    for p in (6666, 6667, 7000):
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.bind(("0.0.0.0", p))
        socks.append(s)
    say("listening")
    while True:
        r, _, _ = select.select(socks, [], [], 30)
        if not r:
            return
        for s in r:
            data, _ = s.recvfrom(4096)
            port = s.getsockname()[1]
            try:
                say("heard", {"port": port, "json": json.loads(decrypt_udp(data))})
            except Exception as e:
                say("undecodable", {"port": port, "error": repr(e), "hex": data.hex()})


if sys.argv[1] == "session":
    session(*sys.argv[2:7])
else:
    listen()

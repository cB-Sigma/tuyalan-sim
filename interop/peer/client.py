# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
#
# tinytuya's client against a simulated device, for interop tests.
#
#   python client.py session <host> <port> <version> <device id> <local key>
#   python client.py listen
#   python client.py gateway <host> <port> <version> <gateway id> <local key> <cid1> <cid2>
#   python client.py bulb|cover <host> <port> <version> <device id> <local key>
#   python client.py devinfo <host> <udp port>
#   python client.py subdev|detect|irblaster|fan|thermostat <host> <port> <version> <device id> <local key>
#
# "session" reads, switches and sets a device with tinytuya's
# OutletDevice and prints "PEER <step> <json>" after each step; after
# "PEER waiting" it reads one status push. "listen" binds UDP 6666, 6667
# and 7000 and prints each announcement as tinytuya decodes it.
# "gateway" drives a plug and a bulb sub-device of a gateway with
# tinytuya's child devices (cid, parent); "bulb" and "cover" drive
# tinytuya's BulbDevice and CoverDevice; "devinfo" sends tinytuya's
# REQ_DEVINFO (scanner.send_discovery_request) and decodes the answer;
# "subdev" sends tinytuya's subdev_query to a gateway; "detect" runs
# tinytuya's detect_available_dps; "irblaster", "fan" and "thermostat"
# drive tinytuya's IRRemoteControlDevice, FloorFanDevice and
# ThermostatDevice (Contrib classes).

import json
import socket
import sys
import select

import tinytuya
from tinytuya.core.udp_helper import decrypt_udp
from tinytuya.scanner import send_discovery_request


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


def opts(version, port):
    return dict(version=float(version), port=int(port), connection_timeout=5, connection_retry_limit=2, connection_retry_delay=1)


def gateway(host, port, version, gw_id, key, cid1, cid2):
    gw = tinytuya.Device(gw_id, host, key, **opts(version, port))
    gw.set_socketPersistent(True)
    plug = tinytuya.OutletDevice("childplug0000001", cid=cid1, parent=gw)
    bulb = tinytuya.BulbDevice("childbulb0000001", cid=cid2, parent=gw)
    if version == "3.2":
        plug.set_dpsUsed({"1": None, "19": None})
        bulb.set_dpsUsed({"20": None, "21": None, "22": None})
    say("status", plug.status())
    say("turn_on", plug.turn_on())
    say("updatedps", plug.updatedps([19]))
    say("status2", plug.status())
    say("bulb", bulb.status())
    gw.close()
    say("done")


def bulb(host, port, version, dev_id, key):
    b = tinytuya.BulbDevice(dev_id, host, key, **opts(version, port))
    b.set_socketPersistent(True)
    say("status", b.status())
    say("type", {"type": b.bulb_type})
    say("set_colour", b.set_colour(10, 20, 200))
    say("set_white_percentage", b.set_white_percentage(60, 30))
    say("state", b.state())
    b.close()
    say("done")


def cover(host, port, version, dev_id, key):
    c = tinytuya.CoverDevice(dev_id, host, key, **opts(version, port))
    c.set_socketPersistent(True)
    say("open", c.open_cover())
    say("position", c.set_value(2, 40))
    say("stop", c.stop_cover())
    say("status", c.status())
    c.close()
    say("done")


def subdev(host, port, version, gw_id, key):
    # tinytuya's sub-device online query, sent to the simulated gateway.
    gw = tinytuya.Device(gw_id, host, key, **opts(version, port))
    say("subdev", gw.subdev_query())
    gw.close()
    say("done")


def detect(host, port, version, dev_id, key):
    # tinytuya's data point brute force against a device22 simulator.
    tinytuya.set_debug(True, False)  # shown by the test only on failure
    d = tinytuya.Device(dev_id, host, key, **opts(version, port))
    d.set_socketPersistent(True)
    # The first run switches to device22 on its first range; a second run
    # starts as device22.
    first = sorted(d.detect_available_dps().keys(), key=int)
    second = sorted(d.detect_available_dps().keys(), key=int)
    say("detect", {"dps": first, "again": second, "dev_type": d.dev_type})
    d.close()
    say("done")


def irblaster(host, port, version, dev_id, key):
    # tinytuya's IRRemoteControlDevice: it detects the control type itself,
    # sends a learned code and a head/key, then learns a button the
    # simulator pushes.
    from tinytuya.Contrib.IRRemoteControlDevice import IRRemoteControlDevice as IR
    d = IR(dev_id, host, key, **opts(version, port))
    say("type", {"type": d.control_type})
    d.set_socketPersistent(True)
    pulses = IR.nec_to_pulses(4, 8)
    head, k = IR.pulses_to_head_key(pulses)
    d.send_button(IR.pulses_to_base64(pulses))
    d.send_key(head, k)
    say("sent", {"head": head, "key": k})
    say("learned", {"code": d.receive_button(timeout=15)})
    d.close()
    say("done")


def fan(host, port, version, dev_id, key):
    from tinytuya.Contrib.FloorFanDevice import FloorFanDevice
    f = FloorFanDevice(dev_id, host, key, **opts(version, port))
    f.set_socketPersistent(True)
    f.set_power(True)
    f.set_mode("nature")
    f.set_speed(3)
    f.set_oscillation(True)
    f.set_timer("4h")
    say("fan", f.status_json())
    f.close()
    say("done")


def thermostat(host, port, version, dev_id, key):
    from tinytuya.Contrib.ThermostatDevice import ThermostatDevice
    t = ThermostatDevice(dev_id, host, key, **opts(version, port))
    t.status()
    say("thermostat", {"high_resolution": t.high_resolution, "cooling_c": t.cooling_setpoint_c, "temperature_c": t.temperature_c,
                       "sensors": [{"id": s.id, "name": s.name, "temperature": s.temperature, "battery": s.battery} for s in t.sensors]})
    t.setCoolSetpoint(23)
    t.sensors["Hall"].setName("Landing")
    t.status()
    say("after", {"cooling_c": t.cooling_setpoint_c, "names": [s.name for s in t.sensors]})
    t.close()
    say("done")


def devinfo(host, port):
    # tinytuya's own REQ_DEVINFO sender, aimed at the simulator; the answer
    # comes back to this socket.
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind(("0.0.0.0", 0))
    s.settimeout(5)
    me = socket.gethostbyname(socket.gethostname())
    send_discovery_request({me: {"broadcast": host, "port": int(port), "socket": s}})
    data, _ = s.recvfrom(4096)
    say("devinfo", {"json": json.loads(decrypt_udp(data))})


mode = sys.argv[1]
if mode == "session":
    session(*sys.argv[2:7])
elif mode == "gateway":
    gateway(*sys.argv[2:9])
elif mode == "bulb":
    bulb(*sys.argv[2:7])
elif mode == "cover":
    cover(*sys.argv[2:7])
elif mode == "devinfo":
    devinfo(*sys.argv[2:4])
elif mode == "subdev":
    subdev(*sys.argv[2:7])
elif mode == "detect":
    detect(*sys.argv[2:7])
elif mode == "irblaster":
    irblaster(*sys.argv[2:7])
elif mode == "fan":
    fan(*sys.argv[2:7])
elif mode == "thermostat":
    thermostat(*sys.argv[2:7])
else:
    listen()

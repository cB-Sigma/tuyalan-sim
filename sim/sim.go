// Package sim simulates Tuya devices on the LAN protocol for tests and
// demos: a TCP server per device speaking protocol 3.1 to 3.5 (session key
// negotiation for 3.4 and 3.5, DP_QUERY, CONTROL and CONTROL_NEW with
// their acknowledgements, UPDATE_DPS, status pushes, heartbeats), gateway
// sub-devices addressed by cid (AddSub), the UDP discovery announcement
// and answers to REQ_DEVINFO (ServeDevInfo), and fault switches (refused
// or dropped controls, the network going down). The data points live
// behind the Backend interface, so a test can keep them wherever its
// other simulators do; Memory is a ready one, and NewPlug, NewBulb,
// NewCover, NewFan, NewClimate, NewThermostat and NewIRBlaster give the
// data points of common devices. Built from public descriptions of the
// protocol; not a Tuya product.
package sim

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/cB-Sigma/tuyalan"
)

// Backend holds a simulated device's data points, keyed by data point ID,
// with values in their LAN form (booleans, numbers, strings; colours as
// hex text).
type Backend interface {
	// Read returns the values of the data points ids, or of every data
	// point when ids is nil.
	Read(ids []string) map[string]any
	// Write applies a CONTROL. It returns the values that changed, which
	// are pushed to every session, or ok false to refuse the whole control
	// as malformed (nothing changed).
	Write(dps map[string]any) (changed map[string]any, ok bool)
}

// Config describes a simulated device.
type Config struct {
	ID       string // device ID (gwId)
	LocalKey string // 16 characters
	Version  string // 3.1 to 3.5; "" is 3.3
	// ProductKey is announced in discovery.
	ProductKey string
	// Device22 makes the device answer DP_QUERY with "data unvalid", as
	// some 3.3 devices do; 3.2 devices always do.
	Device22 bool
}

// Device is a simulated device.
type Device struct {
	cfg     Config
	backend Backend

	mu              sync.Mutex
	srv             *server
	controlRet      uint32
	noControlAck    bool
	subs            map[string]Backend
	subOrder        []string
	subOffline      map[string]bool
	devinfo         *net.UDPConn
	devInfoRequests int
}

// New returns a device whose data points are kept by b.
func New(cfg Config, b Backend) *Device {
	if cfg.Version == "" {
		cfg.Version = "3.3"
	}
	return &Device{cfg: cfg, backend: b}
}

// Config returns the device's configuration.
func (d *Device) Config() Config { return d.cfg }

// SetControlRet refuses every CONTROL with that return code and the text
// "data format error", changing nothing (0: accept).
func (d *Device) SetControlRet(ret uint32) {
	d.mu.Lock()
	d.controlRet = ret
	d.mu.Unlock()
}

// SetNoControlAck drops CONTROL frames unanswered while true (a command
// lost on the way); status pushes go on.
func (d *Device) SetNoControlAck(drop bool) {
	d.mu.Lock()
	d.noControlAck = drop
	d.mu.Unlock()
}

type server struct {
	d  *Device
	ln net.Listener

	mu       sync.Mutex
	sessions map[*session]bool
	down     bool
	accepted int
}

type session struct {
	c     net.Conn
	mu    sync.Mutex
	codec tuyalan.Codec
	ready bool // negotiated (always for 3.1 to 3.3)
	seq   uint32
}

// Start serves the device's LAN protocol on addr (host:port; port 0 for
// a free one, or a stopped device's address to bring it back there) and
// returns the address it listens on.
func (d *Device) Start(addr string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	s := &server{d: d, ln: ln, sessions: map[*session]bool{}}
	d.mu.Lock()
	d.srv = s
	d.mu.Unlock()
	go s.accept()
	return ln.Addr().String(), nil
}

// Addr is the LAN server's address ("" when not started).
func (d *Device) Addr() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.srv == nil {
		return ""
	}
	return d.srv.ln.Addr().String()
}

// Port returns the LAN server's port (0 when not started).
func (d *Device) Port() int {
	_, p, _ := net.SplitHostPort(d.Addr())
	n, _ := strconv.Atoi(p)
	return n
}

// Stop closes the LAN server and its sessions, and the REQ_DEVINFO
// listener.
func (d *Device) Stop() {
	d.mu.Lock()
	s := d.srv
	d.srv = nil
	u := d.devinfo
	d.devinfo = nil
	d.mu.Unlock()
	if u != nil {
		u.Close()
	}
	if s == nil {
		return
	}
	s.ln.Close()
	s.kick()
}

// AddSub makes the device a gateway with a sub-device of node ID cid
// whose data points b keeps. Requests that carry the cid (top-level or in
// "data") read and set the sub-device; its answers and pushes carry the
// cid. Requests without a cid are the gateway's own.
func (d *Device) AddSub(cid string, b Backend) {
	d.mu.Lock()
	if d.subs == nil {
		d.subs = map[string]Backend{}
		d.subOffline = map[string]bool{}
	}
	if _, ok := d.subs[cid]; !ok {
		d.subOrder = append(d.subOrder, cid)
	}
	d.subs[cid] = b
	d.mu.Unlock()
}

// SetSubOffline marks a sub-device online or offline for the answer to a
// SubdevQuery (LAN_EXT_STREAM). Sub-devices are online by default.
func (d *Device) SetSubOffline(cid string, offline bool) {
	d.mu.Lock()
	if d.subOffline == nil {
		d.subOffline = map[string]bool{}
	}
	d.subOffline[cid] = offline
	d.mu.Unlock()
}

// subdevStatus returns the online and offline sub-device cids in the order
// they were added.
func (d *Device) subdevStatus() (online, offline []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, cid := range d.subOrder {
		if d.subOffline[cid] {
			offline = append(offline, cid)
		} else {
			online = append(online, cid)
		}
	}
	return online, offline
}

// PushSub sends changed data points of a sub-device to every negotiated
// session, as a gateway does after a change made at the sub-device.
func (d *Device) PushSub(cid string, dps map[string]any) {
	d.mu.Lock()
	s := d.srv
	d.mu.Unlock()
	if s != nil {
		s.pushCID(dps, cid)
	}
}

// backendFor returns the backend of the sub-device cid, or the device's
// own for "". ok is false for an unknown cid.
func (d *Device) backendFor(cid string) (Backend, bool) {
	if cid == "" {
		return d.backend, true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	b, ok := d.subs[cid]
	return b, ok
}

// SetDown drops every session and refuses new ones while down (the device
// went off the network).
func (d *Device) SetDown(down bool) {
	d.mu.Lock()
	s := d.srv
	d.mu.Unlock()
	if s == nil {
		return
	}
	s.mu.Lock()
	s.down = down
	s.mu.Unlock()
	if down {
		s.kick()
	}
}

// Sessions is the number of LAN sessions accepted so far.
func (d *Device) Sessions() int {
	d.mu.Lock()
	s := d.srv
	d.mu.Unlock()
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted
}

// Push sends changed data points (LAN form) to every negotiated session
// as a status push, as the device does after a change made at the device.
func (d *Device) Push(dps map[string]any) {
	d.mu.Lock()
	s := d.srv
	d.mu.Unlock()
	if s != nil {
		s.push(dps)
	}
}

func (s *server) kick() {
	s.mu.Lock()
	for ss := range s.sessions {
		ss.c.Close()
	}
	s.mu.Unlock()
}

func (s *server) accept() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.down {
			s.mu.Unlock()
			c.Close()
			continue
		}
		ss := &session{c: c, codec: tuyalan.Codec{Version: s.d.cfg.Version, Key: []byte(s.d.cfg.LocalKey)}, ready: tuyalan.Legacy(s.d.cfg.Version)}
		s.sessions[ss] = true
		s.accepted++
		s.mu.Unlock()
		go s.serve(ss)
	}
}

func (ss *session) send(cmd uint32, plain []byte) error {
	ss.mu.Lock()
	ss.seq++
	seq := ss.seq
	ss.mu.Unlock()
	return ss.reply(cmd, seq, 0, plain)
}

// reply sends a frame with a sequence number (a request's, for its
// answer) and a return code. An answer without content (a control's
// acknowledgement, a heartbeat) has an empty payload, neither encrypted
// nor with a version header, as devices send it and as tinytuya expects.
func (ss *session) reply(cmd, seq, ret uint32, plain []byte) error {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	var p []byte
	if len(plain) > 0 {
		var err error
		if p, err = ss.codec.SealPayload(cmd, plain); err != nil {
			return err
		}
	}
	b, err := ss.codec.Frame(tuyalan.Message{Seq: seq, Cmd: cmd, HasRet: true, Ret: ret, Payload: p})
	if err != nil {
		return err
	}
	_, err = ss.c.Write(b)
	return err
}

// statusPayload is a status push of dps; a sub-device's carries its cid
// beside "dps" (3.1 to 3.3) or in "data" (3.4, 3.5).
func (s *server) statusPayload(dps map[string]any, cid string) []byte {
	if tuyalan.Legacy(s.d.cfg.Version) {
		m := map[string]any{"devId": s.d.cfg.ID, "dps": dps, "t": time.Now().Unix()}
		if cid != "" {
			m["cid"] = cid
		}
		b, _ := json.Marshal(m)
		return b
	}
	data := map[string]any{"dps": dps}
	if cid != "" {
		data["cid"] = cid
	}
	b, _ := json.Marshal(map[string]any{"protocol": 4, "t": time.Now().Unix(), "data": data})
	return b
}

// push sends changed data points to every negotiated session.
func (s *server) push(dps map[string]any) { s.pushCID(dps, "") }

func (s *server) pushCID(dps map[string]any, cid string) {
	s.mu.Lock()
	var list []*session
	for ss := range s.sessions {
		list = append(list, ss)
	}
	s.mu.Unlock()
	p := s.statusPayload(dps, cid)
	for _, ss := range list {
		ss.mu.Lock()
		ready := ss.ready
		ss.mu.Unlock()
		if ready {
			_ = ss.send(tuyalan.CmdStatus, p)
		}
	}
}

func (s *server) serve(ss *session) {
	defer func() {
		ss.c.Close()
		s.mu.Lock()
		delete(s.sessions, ss)
		s.mu.Unlock()
	}()
	localKey := []byte(s.d.cfg.LocalKey)
	var devNonce, clientNonce []byte
	for {
		b, err := tuyalan.ReadFrame(ss.c)
		if err != nil {
			return
		}
		ss.mu.Lock()
		codec := ss.codec
		ss.mu.Unlock()
		m, err := codec.Parse(b, false)
		if err != nil {
			return // a wrong key: the device drops the session
		}
		p, err := codec.OpenPayload(m.Cmd, m.Payload)
		if err != nil {
			return
		}
		switch m.Cmd {
		case tuyalan.CmdSessKeyStart:
			if len(p) < 16 {
				return
			}
			clientNonce = append([]byte(nil), p[:16]...)
			devNonce = make([]byte, 16)
			_, _ = rand.Read(devNonce)
			resp := append(append([]byte(nil), devNonce...), tuyalan.HMACSHA256(localKey, clientNonce)...)
			if ss.send(tuyalan.CmdSessKeyResp, resp) != nil {
				return
			}
		case tuyalan.CmdSessKeyFinish:
			if devNonce == nil || !bytes.Equal(p, tuyalan.HMACSHA256(localKey, devNonce)) {
				return
			}
			sk, err := tuyalan.SessionKey(s.d.cfg.Version, localKey, clientNonce, devNonce)
			if err != nil {
				return
			}
			ss.mu.Lock()
			ss.codec.Key, ss.ready = sk, true
			ss.mu.Unlock()
		case tuyalan.CmdHeartBeat:
			_ = ss.send(tuyalan.CmdHeartBeat, nil)
		case tuyalan.CmdDPQuery, tuyalan.CmdDPQueryNew:
			cid := tuyalan.CID(p)
			// 3.2 devices ("device22") answer DP_QUERY so.
			if cid == "" && (s.d.cfg.Device22 || s.d.cfg.Version == "3.2") && m.Cmd == tuyalan.CmdDPQuery {
				_ = ss.send(m.Cmd, []byte("json obj data unvalid"))
				continue
			}
			be, ok := s.d.backendFor(cid)
			if !ok {
				_ = ss.reply(m.Cmd, m.Seq, 1, []byte("data format error"))
				continue
			}
			ans := map[string]any{"devId": s.d.cfg.ID, "dps": be.Read(nil), "t": time.Now().Unix()}
			if cid != "" {
				ans["cid"] = cid
			}
			b, _ := json.Marshal(ans)
			_ = ss.send(m.Cmd, b)
		case tuyalan.CmdUpdateDPs:
			// Acknowledged, then the asked data points are pushed to this
			// session.
			cid := tuyalan.CID(p)
			be, ok := s.d.backendFor(cid)
			var req struct {
				DPID []json.Number `json:"dpId"`
			}
			if !ok || json.Unmarshal(p, &req) != nil {
				_ = ss.reply(m.Cmd, m.Seq, 1, []byte("data format error"))
				continue
			}
			_ = ss.reply(m.Cmd, m.Seq, 0, nil)
			ids := make([]string, 0, len(req.DPID))
			for _, n := range req.DPID {
				ids = append(ids, n.String())
			}
			if vals := be.Read(ids); len(vals) > 0 {
				_ = ss.send(tuyalan.CmdStatus, s.statusPayload(vals, cid))
			}
		case tuyalan.CmdLanExtStream:
			// A gateway's extension request; the sub-device online query is
			// the one answered. 3.3 gateways do not answer it, so only 3.4
			// and 3.5 devices reply.
			var req struct {
				ReqType string `json:"reqType"`
			}
			if tuyalan.Legacy(s.d.cfg.Version) || json.Unmarshal(p, &req) != nil || req.ReqType != "subdev_online_stat_query" {
				continue
			}
			online, offline := s.d.subdevStatus()
			if online == nil {
				online = []string{}
			}
			if offline == nil {
				offline = []string{}
			}
			body, _ := json.Marshal(map[string]any{"data": map[string]any{"online": online, "offline": offline}})
			_ = ss.reply(tuyalan.CmdLanExtStream, m.Seq, 0, body)
		case tuyalan.CmdControl, tuyalan.CmdControlNew:
			s.d.mu.Lock()
			refuse, drop := s.d.controlRet, s.d.noControlAck
			s.d.mu.Unlock()
			if drop {
				continue
			}
			dps, ok := tuyalan.DPS(p)
			cid := tuyalan.CID(p)
			be, known := s.d.backendFor(cid)
			if !known {
				ok = false
			}
			if !ok || refuse != 0 {
				if refuse == 0 {
					refuse = 1
				}
				_ = ss.reply(m.Cmd, m.Seq, refuse, []byte("data format error"))
				continue
			}
			// A null value asks for the data point's value (the 3.2 form
			// of a query).
			var query []string
			set := map[string]any{}
			for id, v := range dps {
				if v == nil {
					query = append(query, id)
				} else {
					set[id] = v
				}
			}
			changed, ok := be.Write(set)
			if !ok {
				_ = ss.reply(m.Cmd, m.Seq, 1, []byte("data format error"))
				continue
			}
			_ = ss.reply(m.Cmd, m.Seq, 0, nil)
			if len(query) > 0 {
				_ = ss.send(tuyalan.CmdStatus, s.statusPayload(be.Read(query), cid))
			}
			if len(changed) > 0 {
				s.pushCID(changed, cid)
			}
		}
	}
}

// Broadcast sends the device's discovery announcement to addr (host:port),
// in the form of its version.
func (d *Device) Broadcast(addr string) error {
	b, err := d.announcement()
	if err != nil {
		return err
	}
	c, err := net.Dial("udp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.Write(b)
	return err
}

func (d *Device) announcement() ([]byte, error) {
	host := "127.0.0.1"
	if a := d.Addr(); a != "" {
		host, _, _ = net.SplitHostPort(a)
	}
	return tuyalan.EncodeBroadcast(tuyalan.Broadcast{IP: host, GwID: d.cfg.ID, Active: 2, Encrypt: d.cfg.Version != "3.1", ProductKey: d.cfg.ProductKey, Version: d.cfg.Version})
}

// ServeDevInfo answers REQ_DEVINFO requests on the UDP address addr
// (":7000" as a device; port 0 for a free one) until Stop: each valid
// request gets the device's announcement, in the form of its version,
// sent to the request's source address. Real devices that answer are
// 3.5 devices; the simulator answers in any version. It returns the
// address it listens on.
func (d *Device) ServeDevInfo(addr string) (string, error) {
	ua, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return "", err
	}
	c, err := net.ListenUDP("udp", ua)
	if err != nil {
		return "", err
	}
	d.mu.Lock()
	old := d.devinfo
	d.devinfo = c
	d.mu.Unlock()
	if old != nil {
		old.Close()
	}
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := c.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if _, err := tuyalan.ParseDevInfoRequest(buf[:n]); err != nil {
				continue
			}
			d.mu.Lock()
			d.devInfoRequests++
			d.mu.Unlock()
			if b, err := d.announcement(); err == nil {
				_, _ = c.WriteToUDP(b, from)
			}
		}
	}()
	return c.LocalAddr().String(), nil
}

// DevInfoRequests is the number of valid REQ_DEVINFO requests received.
func (d *Device) DevInfoRequests() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.devInfoRequests
}

// Memory is a Backend that keeps the data points in memory and records
// every control.
type Memory struct {
	mu       sync.Mutex
	dps      map[string]any
	controls []map[string]any
}

// NewMemory returns a Memory holding a copy of dps.
func NewMemory(dps map[string]any) *Memory {
	m := &Memory{dps: map[string]any{}}
	for k, v := range dps {
		m.dps[k] = v
	}
	return m
}

// Read implements Backend.
func (m *Memory) Read(ids []string) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]any{}
	for k, v := range m.dps {
		if ids == nil {
			out[k] = v
		}
	}
	for _, id := range ids {
		if v, ok := m.dps[id]; ok {
			out[id] = v
		}
	}
	return out
}

// Write implements Backend: values of known data points are set; unknown
// IDs are ignored, as devices do.
func (m *Memory) Write(dps map[string]any) (map[string]any, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := map[string]any{}
	for id, v := range dps {
		if _, ok := m.dps[id]; ok {
			m.dps[id] = v
			changed[id] = v
		}
	}
	if len(changed) > 0 {
		m.controls = append(m.controls, changed)
	}
	return changed, true
}

// Set changes a data point as the device itself would; push it to the
// sessions with Device.Push.
func (m *Memory) Set(id string, v any) {
	m.mu.Lock()
	m.dps[id] = v
	m.mu.Unlock()
}

// Controls returns the controls received (data point ID to value), in
// order.
func (m *Memory) Controls() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]any(nil), m.controls...)
}

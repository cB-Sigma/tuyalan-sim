// Package sim simulates Tuya devices on the LAN protocol for tests and
// demos: a TCP server per device speaking protocol 3.1 to 3.5 (session key
// negotiation for 3.4 and 3.5, DP_QUERY, CONTROL and CONTROL_NEW with
// their acknowledgements, status pushes, heartbeats), the UDP discovery
// announcement, and fault switches (refused or dropped controls, the
// network going down). The data points live behind the Backend interface,
// so a test can keep them wherever its other simulators do; Memory is a
// ready one. Built from public descriptions of the protocol; not a Tuya
// product.
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

	mu           sync.Mutex
	srv          *server
	controlRet   uint32
	noControlAck bool
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

// Stop closes the LAN server and its sessions.
func (d *Device) Stop() {
	d.mu.Lock()
	s := d.srv
	d.srv = nil
	d.mu.Unlock()
	if s == nil {
		return
	}
	s.ln.Close()
	s.kick()
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

func (s *server) statusPayload(dps map[string]any) []byte {
	if tuyalan.Legacy(s.d.cfg.Version) {
		b, _ := json.Marshal(map[string]any{"devId": s.d.cfg.ID, "dps": dps, "t": time.Now().Unix()})
		return b
	}
	b, _ := json.Marshal(map[string]any{"protocol": 4, "t": time.Now().Unix(), "data": map[string]any{"dps": dps}})
	return b
}

// push sends changed data points to every negotiated session.
func (s *server) push(dps map[string]any) {
	s.mu.Lock()
	var list []*session
	for ss := range s.sessions {
		list = append(list, ss)
	}
	s.mu.Unlock()
	p := s.statusPayload(dps)
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
			// 3.2 devices ("device22") answer DP_QUERY so.
			if (s.d.cfg.Device22 || s.d.cfg.Version == "3.2") && m.Cmd == tuyalan.CmdDPQuery {
				_ = ss.send(m.Cmd, []byte("json obj data unvalid"))
				continue
			}
			b, _ := json.Marshal(map[string]any{"devId": s.d.cfg.ID, "dps": s.d.backend.Read(nil), "t": time.Now().Unix()})
			_ = ss.send(m.Cmd, b)
		case tuyalan.CmdControl, tuyalan.CmdControlNew:
			s.d.mu.Lock()
			refuse, drop := s.d.controlRet, s.d.noControlAck
			s.d.mu.Unlock()
			if drop {
				continue
			}
			dps, ok := tuyalan.DPS(p)
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
			changed, ok := s.d.backend.Write(set)
			if !ok {
				_ = ss.reply(m.Cmd, m.Seq, 1, []byte("data format error"))
				continue
			}
			_ = ss.reply(m.Cmd, m.Seq, 0, nil)
			if len(query) > 0 {
				_ = ss.send(tuyalan.CmdStatus, s.statusPayload(s.d.backend.Read(query)))
			}
			if len(changed) > 0 {
				s.push(changed)
			}
		}
	}
}

// Broadcast sends the device's discovery announcement to addr (host:port),
// in the form of its version.
func (d *Device) Broadcast(addr string) error {
	host := "127.0.0.1"
	if a := d.Addr(); a != "" {
		host, _, _ = net.SplitHostPort(a)
	}
	b, err := tuyalan.EncodeBroadcast(tuyalan.Broadcast{IP: host, GwID: d.cfg.ID, Active: 2, Encrypt: d.cfg.Version != "3.1", ProductKey: d.cfg.ProductKey, Version: d.cfg.Version})
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

// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package sim

import (
	"net"
	"testing"

	"github.com/cB-Sigma/tuyalan"
)

// A control's acknowledgement and a heartbeat answer carry an empty
// payload in every version: tinytuya takes only an empty payload for an
// acknowledgement and reported an encrypted empty one as invalid JSON
// (found by the tinytuya interop test).
func TestEmptyAnswersHaveEmptyPayloads(t *testing.T) {
	for _, v := range tuyalan.AllVersions {
		for _, cmd := range []uint32{tuyalan.CmdControl, tuyalan.CmdControlNew, tuyalan.CmdHeartBeat} {
			a, b := net.Pipe()
			ss := &session{c: a, codec: tuyalan.Codec{Version: v, Key: []byte("0123456789abcdef")}}
			go func() { _ = ss.reply(cmd, 7, 0, nil); a.Close() }()
			raw, err := tuyalan.ReadFrame(b)
			b.Close()
			if err != nil {
				t.Fatalf("%s cmd %#x: %v", v, cmd, err)
			}
			m, err := ss.codec.Parse(raw, true)
			if err != nil || m.Seq != 7 || m.Cmd != cmd || !m.HasRet || m.Ret != 0 || len(m.Payload) != 0 {
				t.Errorf("%s cmd %#x: %+v %v", v, cmd, m, err)
			}
		}
	}
}

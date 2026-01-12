package roon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// SOOD is Roon Core discovery protocol used by node-roon-api (sood.js).
// Packets start with "SOOD", version 2, then a 1-byte type (e.g. 'Q').

const (
	soodPort        = 9003
	soodMulticastIP = "239.255.90.90"
	soodVersion     = 2
)

type soodMessage struct {
	Type  byte
	Props map[string]string // nil values in JS are encoded as length 0xFFFF; we treat those as absent.

	FromIP   string
	FromPort int
}

func encodeSoodQuery(props map[string]string) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("SOOD")
	b.WriteByte(soodVersion)
	b.WriteByte('Q')

	for k, v := range props {
		if k == "" {
			return nil, errors.New("sood: empty key")
		}
		if len(k) > 255 {
			return nil, fmt.Errorf("sood: key too long: %q", k)
		}

		b.WriteByte(byte(len(k)))
		b.WriteString(k)

		if v == "" {
			// Empty string is valid and encoded with length 0.
			_ = binary.Write(&b, binary.BigEndian, uint16(0))
			continue
		}

		if len(v) > 65534 {
			return nil, fmt.Errorf("sood: value too long for key %q", k)
		}
		_ = binary.Write(&b, binary.BigEndian, uint16(len(v)))
		b.WriteString(v)
	}

	return b.Bytes(), nil
}

func parseSoodPacket(p []byte, fromIP string, fromPort int) (*soodMessage, error) {
	if len(p) < 6 {
		return nil, errors.New("sood: packet too short")
	}
	if string(p[0:4]) != "SOOD" {
		return nil, errors.New("sood: bad magic")
	}
	if p[4] != soodVersion {
		return nil, fmt.Errorf("sood: unsupported version %d", p[4])
	}

	m := &soodMessage{
		Type:     p[5],
		Props:    map[string]string{},
		FromIP:   fromIP,
		FromPort: fromPort,
	}

	pos := 6
	for pos < len(p) {
		namelen := int(p[pos])
		pos++
		if namelen == 0 {
			return nil, errors.New("sood: zero name len")
		}
		if pos+namelen > len(p) {
			return nil, errors.New("sood: truncated name")
		}
		name := string(p[pos : pos+namelen])
		pos += namelen

		if pos+2 > len(p) {
			return nil, errors.New("sood: truncated value len")
		}
		vlen := int(binary.BigEndian.Uint16(p[pos : pos+2]))
		pos += 2

		// 0xFFFF is "null" in JS implementation. We'll treat it as absent.
		if vlen == 0xFFFF {
			continue
		}
		if vlen == 0 {
			m.Props[name] = ""
			continue
		}
		if pos+vlen > len(p) {
			return nil, errors.New("sood: truncated value")
		}
		m.Props[name] = string(p[pos : pos+vlen])
		pos += vlen
	}

	// JS implementation supports overriding reply address/port.
	if ra, ok := m.Props["_replyaddr"]; ok && ra != "" {
		delete(m.Props, "_replyaddr")
		m.FromIP = ra
	}
	if rp, ok := m.Props["_replyport"]; ok && rp != "" {
		delete(m.Props, "_replyport")
		// Best effort parse; leave as-is on failure.
		var n int
		_, _ = fmt.Sscanf(rp, "%d", &n)
		if n > 0 && n < 65536 {
			m.FromPort = n
		}
	}

	return m, nil
}

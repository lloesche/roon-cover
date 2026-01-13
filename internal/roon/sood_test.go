package roon

import (
	"strings"
	"testing"
)

func TestSoodEncodeParse_Query(t *testing.T) {
	t.Parallel()

	p, err := encodeSoodQuery(map[string]string{
		"_tid":             "abc",
		"query_service_id": "svc",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	m, err := parseSoodPacket(p, "1.2.3.4", 1234)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Type != 'Q' {
		t.Fatalf("type: got=%q", m.Type)
	}
	if m.Props["_tid"] != "abc" || m.Props["query_service_id"] != "svc" {
		t.Fatalf("props mismatch: %#v", m.Props)
	}
}

func TestSoodEncode_Errors(t *testing.T) {
	t.Parallel()

	if _, err := encodeSoodQuery(map[string]string{"": "x"}); err == nil {
		t.Fatalf("expected error for empty key")
	}

	tooLongKey := strings.Repeat("a", 256)
	if _, err := encodeSoodQuery(map[string]string{tooLongKey: "x"}); err == nil {
		t.Fatalf("expected error for too-long key")
	}

	tooLongVal := strings.Repeat("b", 65535)
	if _, err := encodeSoodQuery(map[string]string{"k": tooLongVal}); err == nil {
		t.Fatalf("expected error for too-long value")
	}
}

func TestSoodParse_ErrorsAndSpecials(t *testing.T) {
	t.Parallel()

	if _, err := parseSoodPacket([]byte(""), "1.2.3.4", 1); err == nil {
		t.Fatalf("expected error for short packet")
	}
	if _, err := parseSoodPacket([]byte("NOPE\x02Q"), "1.2.3.4", 1); err == nil {
		t.Fatalf("expected error for bad magic")
	}
	if _, err := parseSoodPacket([]byte("SOOD\x03Q"), "1.2.3.4", 1); err == nil {
		t.Fatalf("expected error for bad version")
	}

	// null value (0xFFFF) should be treated as absent.
	p, err := encodeSoodQuery(map[string]string{
		"_replyaddr": "9.9.9.9",
		"_replyport": "9999",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Append a property with "null" value: name len=1, name="x", vlen=0xFFFF.
	p = append(p, 1, 'x', 0xFF, 0xFF)

	m, err := parseSoodPacket(p, "1.2.3.4", 1234)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.FromIP != "9.9.9.9" || m.FromPort != 9999 {
		t.Fatalf("reply override got=%s:%d", m.FromIP, m.FromPort)
	}
	if _, ok := m.Props["x"]; ok {
		t.Fatalf("expected null-valued prop to be absent")
	}
}

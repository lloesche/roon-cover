package roon

import "testing"

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

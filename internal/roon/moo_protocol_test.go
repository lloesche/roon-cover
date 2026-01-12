package roon

import (
	"testing"
)

func TestMooEncodeParse_RequestWithJSON(t *testing.T) {
	t.Parallel()

	b, err := encodeMooRequest(7, "com.roonlabs.registry:1/info", map[string]string{"hello": "world"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	f, err := parseMooFrame(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if f.Verb != mooVerbRequest {
		t.Fatalf("verb: got=%v", f.Verb)
	}
	if f.Service != "com.roonlabs.registry:1" || f.Name != "info" {
		t.Fatalf("target: got=%s/%s", f.Service, f.Name)
	}
	if f.RequestID != "7" {
		t.Fatalf("request id: got=%q", f.RequestID)
	}
	if f.ContentType != "application/json" {
		t.Fatalf("content type: got=%q", f.ContentType)
	}
	if f.ContentLength <= 0 {
		t.Fatalf("content length: got=%d", f.ContentLength)
	}
}

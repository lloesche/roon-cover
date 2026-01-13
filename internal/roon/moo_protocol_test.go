package roon

import (
	"encoding/json"
	"strings"
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

func TestParseMooFrame_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		buf  []byte
		want string
	}{
		{name: "empty", buf: nil, want: "moo: empty frame"},
		{name: "missing_newline", buf: []byte("MOO/1 REQUEST svc/name"), want: "moo: missing first line newline"},
		{name: "bad_magic", buf: []byte("NOPE/1 REQUEST svc/name\nRequest-Id: 1\n\n"), want: "moo: bad first line"},
		{name: "bad_first_line_format", buf: []byte("MOO/1 REQUEST\n"), want: "moo: bad first line format"},
		{name: "unsupported_verb", buf: []byte("MOO/1 WAT svc/name\nRequest-Id: 1\n\n"), want: "moo: unsupported verb"},
		{name: "bad_request_target", buf: []byte("MOO/1 REQUEST svc-only\nRequest-Id: 1\n\n"), want: "moo: bad request target"},
		{name: "truncated_headers", buf: []byte("MOO/1 REQUEST svc/name\nRequest-Id: 1\n"), want: "moo: truncated headers"},
		{name: "bad_header_line", buf: []byte("MOO/1 REQUEST svc/name\nRequest-Id: 1\nNoColonHere\n\n"), want: "moo: bad header line"},
		{name: "missing_request_id", buf: []byte("MOO/1 REQUEST svc/name\nContent-Length: 0\n\n"), want: "moo: missing Request-Id"},
		{name: "bad_content_length", buf: []byte("MOO/1 REQUEST svc/name\nRequest-Id: 1\nContent-Length: -1\n\n"), want: "moo: bad content-length"},
		{name: "length_without_type", buf: []byte("MOO/1 REQUEST svc/name\nRequest-Id: 1\nContent-Length: 1\n\nx"), want: "moo: Content-Length without Content-Type"},
		{name: "truncated_body", buf: []byte("MOO/1 REQUEST svc/name\nRequest-Id: 1\nContent-Length: 5\nContent-Type: application/json\n\n{}"), want: "moo: truncated body"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseMooFrame(tc.buf)
			if err == nil {
				t.Fatalf("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%q want contains %q", err.Error(), tc.want)
			}
		})
	}
}

func TestEncodeMooFrame_BodyVariants(t *testing.T) {
	t.Parallel()

	// nil body: no content headers
	b, err := encodeMooRequest(1, "svc/name", nil)
	if err != nil {
		t.Fatalf("encode nil: %v", err)
	}
	s := string(b)
	if strings.Contains(strings.ToLower(s), "content-length") || strings.Contains(strings.ToLower(s), "content-type") {
		t.Fatalf("unexpected content headers in %q", s)
	}

	// json.RawMessage body: stays json
	raw := json.RawMessage(`{"x":1}`)
	b, err = encodeMooRequest(2, "svc/name", raw)
	if err != nil {
		t.Fatalf("encode raw: %v", err)
	}
	f, err := parseMooFrame(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if f.ContentType != "application/json" {
		t.Fatalf("content-type got=%q", f.ContentType)
	}
	if string(f.BodyRaw) != string(raw) {
		t.Fatalf("body got=%q want=%q", string(f.BodyRaw), string(raw))
	}
}

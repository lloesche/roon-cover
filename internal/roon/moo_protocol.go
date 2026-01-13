package roon

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type mooVerb string

const (
	mooVerbRequest  mooVerb = "REQUEST"
	mooVerbContinue mooVerb = "CONTINUE"
	mooVerbComplete mooVerb = "COMPLETE"
)

type mooFrame struct {
	Verb mooVerb

	// For REQUEST frames only:
	Service string
	Name    string

	// For CONTINUE/COMPLETE frames:
	ResponseName string

	RequestID string

	ContentType   string
	ContentLength int

	Headers map[string]string

	BodyRaw []byte
}

func parseMooFrame(buf []byte) (*mooFrame, error) {
	if len(buf) == 0 {
		return nil, errors.New("moo: empty frame")
	}

	r := bufio.NewReader(bytes.NewReader(buf))

	firstLine, err := r.ReadString('\n')
	if err != nil {
		return nil, errors.New("moo: missing first line newline")
	}
	firstLine = strings.TrimSuffix(firstLine, "\n")

	if !strings.HasPrefix(firstLine, "MOO/") {
		return nil, errors.New("moo: bad first line")
	}

	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) != 3 {
		return nil, errors.New("moo: bad first line format")
	}

	verb := mooVerb(parts[1])
	m := &mooFrame{
		Verb:    verb,
		Headers: map[string]string{},
	}

	switch verb {
	case mooVerbRequest:
		svc, name, ok := strings.Cut(parts[2], "/")
		if !ok || svc == "" || name == "" {
			return nil, errors.New("moo: bad request target")
		}
		m.Service = svc
		m.Name = name
	case mooVerbContinue, mooVerbComplete:
		m.ResponseName = parts[2]
	default:
		return nil, fmt.Errorf("moo: unsupported verb %q", parts[1])
	}

	// Headers (until blank line).
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, errors.New("moo: truncated headers")
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			break
		}

		k, v, ok := strings.Cut(line, ":")
		if !ok {
			return nil, errors.New("moo: bad header line")
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		switch strings.ToLower(k) {
		case "request-id":
			m.RequestID = v
		case "content-type":
			m.ContentType = v
		case "content-length":
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return nil, errors.New("moo: bad content-length")
			}
			m.ContentLength = n
		default:
			m.Headers[k] = v
		}
	}

	if m.RequestID == "" {
		return nil, errors.New("moo: missing Request-Id")
	}

	if m.ContentLength > 0 {
		if m.ContentType == "" {
			return nil, errors.New("moo: Content-Length without Content-Type")
		}
		body := make([]byte, m.ContentLength)
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, errors.New("moo: truncated body")
		}
		m.BodyRaw = body
	}

	return m, nil
}

func encodeMooRequest(requestID int64, fullName string, body any) ([]byte, error) {
	return encodeMooFrame(mooVerbRequest, strconv.FormatInt(requestID, 10), fullName, body)
}

func encodeMooContinue(requestID string, name string, body any) ([]byte, error) {
	return encodeMooFrame(mooVerbContinue, requestID, name, body)
}

func encodeMooComplete(requestID string, name string, body any) ([]byte, error) {
	return encodeMooFrame(mooVerbComplete, requestID, name, body)
}

func encodeMooFrame(verb mooVerb, requestID string, target string, body any) ([]byte, error) {
	var b bytes.Buffer

	switch verb {
	case mooVerbRequest:
		// target = "service/name"
		b.WriteString("MOO/1 REQUEST ")
		b.WriteString(target)
		b.WriteByte('\n')
	case mooVerbContinue:
		b.WriteString("MOO/1 CONTINUE ")
		b.WriteString(target)
		b.WriteByte('\n')
	case mooVerbComplete:
		b.WriteString("MOO/1 COMPLETE ")
		b.WriteString(target)
		b.WriteByte('\n')
	default:
		return nil, fmt.Errorf("moo: unsupported verb %q", verb)
	}

	b.WriteString("Request-Id: ")
	b.WriteString(requestID)
	b.WriteByte('\n')

	var bodyBytes []byte
	var contentType string

	if body != nil {
		switch v := body.(type) {
		case []byte:
			return nil, errors.New("moo: raw []byte bodies must include explicit content-type (not implemented)")
		case json.RawMessage:
			bodyBytes = v
			contentType = "application/json"
		default:
			j, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			bodyBytes = j
			contentType = "application/json"
		}

		b.WriteString("Content-Length: ")
		b.WriteString(strconv.Itoa(len(bodyBytes)))
		b.WriteByte('\n')
		b.WriteString("Content-Type: ")
		b.WriteString(contentType)
		b.WriteByte('\n')
	}

	// Blank line ends header.
	b.WriteByte('\n')

	if len(bodyBytes) > 0 {
		b.Write(bodyBytes)
	}

	return b.Bytes(), nil
}

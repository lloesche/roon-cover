package roon

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func (c *Client) FetchImage(ctx context.Context, core Core, key ImageKey, opt ImageFetchOptions) (bytes []byte, mimeType string, err error) {
	if core.Host == "" || core.Port == 0 {
		return nil, "", errors.New("roon: core missing host/port")
	}
	if key == "" {
		return nil, "", errors.New("roon: missing image key")
	}

	u := url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(core.Host, strconv.Itoa(core.Port)),
		Path:   "/api/image/" + url.PathEscape(string(key)),
	}
	q := u.Query()
	if opt.Size > 0 {
		// Using the documented HTTP form:
		// /api/image/<key>?scale=fit&width=<w>&height=<h>&format=image/jpeg
		q.Set("scale", "fit")
		q.Set("width", strconv.Itoa(opt.Size))
		q.Set("height", strconv.Itoa(opt.Size))
		q.Set("format", "image/jpeg")
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return nil, "", fmt.Errorf("roon: image fetch status %d: %s", resp.StatusCode, string(b))
	}

	mimeType = resp.Header.Get("Content-Type")
	const maxBytes = 24 << 20
	if resp.ContentLength > maxBytes {
		return nil, "", errors.New("roon: artwork exceeds 24 MiB")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(b) > maxBytes {
		return nil, "", errors.New("roon: artwork exceeds 24 MiB")
	}
	return b, mimeType, nil
}

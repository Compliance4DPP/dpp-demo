package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// apiClient calls an HTTP/JSON API: POST base+path with a JSON body, JSON response. The signer
// and the gateway both serve their gRPC services this way (grpc-gateway), so errors come as
// {"code": <gRPC code>, "message": "..."}; a proxy in front may answer with anything else.
type apiClient struct {
	name string
	base string // e.g. https://host/grpc/
	auth string // Authorization header, if any
	http *http.Client
}

// maxResponseBytes caps what is read of a response.
const maxResponseBytes = 16 << 20

func (c *apiClient) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	u := strings.TrimRight(c.base, "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.auth != "" {
		req.Header.Set("Authorization", c.auth)
	}
	hc := c.http
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return &upstreamError{Status: resp.StatusCode, Message: errorMessage(b)}
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("response is not the expected JSON: %w", err)
	}
	return nil
}

// upstreamError is a non-2xx response of a dependency.
type upstreamError struct {
	Status  int
	Message string
}

func (e *upstreamError) Error() string {
	s := fmt.Sprintf("HTTP %d %s", e.Status, http.StatusText(e.Status))
	if e.Message == "" {
		return s
	}
	return s + ": " + e.Message
}

// errorMessage returns the message of a grpc-gateway error body, else the start of a plain-text
// body; HTML error pages (e.g. of a proxy) are left out.
func errorMessage(b []byte) string {
	var s struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &s) == nil && s.Message != "" {
		return s.Message
	}
	t := strings.Join(strings.Fields(string(b)), " ")
	if strings.HasPrefix(t, "<") {
		return ""
	}
	if len(t) > 200 {
		t = t[:200] + "…"
	}
	return t
}

// isTimeout reports whether err is a deadline of the request context.
func isTimeout(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

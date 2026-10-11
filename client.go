package dock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// DefaultBaseURL is the control plane used when neither WithBaseURL nor
// DOCK_API_URL is set.
const DefaultBaseURL = "https://api.usedock.io"

const maxResponseBytes = 64 << 20

// Client talks to one Dock control plane. It is safe for concurrent use.
type Client struct {
	// Docks holds the dock lifecycle and exec calls.
	Docks *DocksService
	// Files holds the file helpers, built on exec.
	Files *FilesService
	// Connectors lists, connects and disconnects a workspace's tool connectors.
	Connectors *ConnectorsService
	// Environments reads and updates organization environments.
	Environments *EnvironmentsService
	// Ports exposes dock ports at URLs, private or public.
	Ports *PortsService
	// Images lists the Dockyard image catalog.
	Images *ImagesService
	// Operations reads long-running operations such as deletes.
	Operations *OperationsService

	apiKey  string
	baseURL string
	http    *http.Client
}

// Option configures NewClient.
type Option func(*Client)

// WithAPIKey sets the API key (dock_...). It overrides DOCK_API_KEY.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL sets the control plane origin, without /v1. It overrides
// DOCK_API_URL.
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient sets the HTTP client. The default has no timeout, so use the
// context to bound calls (create, stop and resume return when the work is done).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient returns a client configured from DOCK_API_KEY and DOCK_API_URL,
// then from opts. A missing API key is reported by the first call.
func NewClient(opts ...Option) *Client {
	c := &Client{
		apiKey:  os.Getenv("DOCK_API_KEY"),
		baseURL: os.Getenv("DOCK_API_URL"),
	}
	for _, o := range opts {
		o(c)
	}
	if c.baseURL == "" {
		c.baseURL = DefaultBaseURL
	}
	c.baseURL = strings.TrimRight(c.baseURL, "/")
	if c.http == nil {
		c.http = &http.Client{}
	}
	c.Docks = &DocksService{c: c}
	c.Files = &FilesService{c: c}
	c.Connectors = &ConnectorsService{c: c}
	c.Environments = &EnvironmentsService{c: c}
	c.Ports = &PortsService{c: c}
	c.Images = &ImagesService{c: c}
	c.Operations = &OperationsService{c: c}
	return c
}

// String describes the client without its API key, so logging it with %v or
// %+v is safe.
func (c Client) String() string { return "dock.Client{baseURL: " + c.baseURL + "}" }

// GoString is String, so %#v does not print the API key either.
func (c Client) GoString() string { return c.String() }

// transportError is a failed round trip. Its message has the API key redacted;
// Unwrap returns the cause (for example context.Canceled).
type transportError struct {
	msg string
	err error
}

func (e *transportError) Error() string { return e.msg }
func (e *transportError) Unwrap() error { return e.err }

type envelope struct {
	OK      bool            `json:"ok"`
	Data    json.RawMessage `json:"data"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
}

// do sends one /v1 request and decodes the envelope data into out (when
// non-nil). body is sent as JSON when non-nil.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, headers map[string]string, body, out any) error {
	if c.apiKey == "" {
		return &Error{Code: "missing_api_key", Message: "no API key: set DOCK_API_KEY or use WithAPIKey"}
	}
	u := c.baseURL + "/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("dock: encode request: %w", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return fmt.Errorf("dock: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// Never echo the key, even if a transport error quotes the request, but
		// keep the cause so errors.Is(err, context.DeadlineExceeded) works.
		return &transportError{msg: "dock: " + strings.ReplaceAll(err.Error(), c.apiKey, "***"), err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("dock: read response: %w", err)
	}
	var env envelope
	if jerr := json.Unmarshal(raw, &env); jerr != nil {
		code := fmt.Sprintf("http_%d", resp.StatusCode)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			code = "bad_response"
		}
		return &Error{Code: code, Status: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
	if !env.OK || resp.StatusCode >= 300 {
		code := env.Code
		if code == "" {
			code = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return &Error{Code: code, Status: resp.StatusCode, Message: env.Message}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return &Error{Code: "bad_response", Status: resp.StatusCode, Message: "unexpected response shape: " + err.Error()}
		}
	}
	return nil
}

package client

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// targetUserIDHeader is sent by admin CLI callers to request a specific user_id
// for indexed documents. The server only honours it for admin users in
// multiuser mode.
const targetUserIDHeader = "X-Hister-Target-User-ID"

type Client struct {
	baseURL        string
	httpClient     *http.Client
	userAgent      string
	accessToken    string
	targetUserID   *uint
	allowSensitive bool
}

type Option func(*Client)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

func WithAccessToken(token string) Option {
	return func(c *Client) { c.accessToken = token }
}

func WithAllowSensitive() Option {
	return func(c *Client) { c.allowSensitive = true }
}

// WithTargetUserID instructs the server to index submitted documents under the
// given user ID instead of the authenticated caller's ID. The server only
// honours this for admin users in multiuser mode.
func WithTargetUserID(uid uint) Option {
	return func(c *Client) { c.targetUserID = &uid }
}

func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	detail := strings.TrimSpace(string(body))

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		msg := "authentication required: the server requires an access token"
		if detail != "" {
			msg += " (" + detail + ")"
		}
		return fmt.Errorf("%s\nProvide one with --token / -t or set access_token in your config file", msg)
	case http.StatusForbidden:
		msg := "access denied: the token is invalid or does not have permission for this operation"
		if detail != "" {
			msg += " (" + detail + ")"
		}
		return fmt.Errorf("%s\nCheck the token with --token / -t or verify the user's permissions on the server", msg)
	case http.StatusNotFound:
		msg := "server not reachable at the configured URL"
		if detail != "" {
			msg += ": " + detail
		}
		return fmt.Errorf("%s\nVerify the server address with --server-url / -u", msg)
	case http.StatusNotAcceptable:
		msg := "page skipped: this URL was rejected by the server (usually due to skip rules or disabled domains)"
		if detail != "" {
			msg += " (" + detail + ")"
		}
		return fmt.Errorf("%s", msg)
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		msg := fmt.Sprintf("server error (%d)", resp.StatusCode)
		if detail != "" {
			msg += ": " + detail
		}
		return fmt.Errorf("%s\nCheck the server logs for details", msg)
	default:
		if detail == "" {
			detail = resp.Status
		}
		return fmt.Errorf("unexpected response (%d): %s", resp.StatusCode, detail)
	}
}

func closeBody(resp *http.Response, errp *error) {
	if cerr := resp.Body.Close(); cerr != nil && *errp == nil {
		*errp = fmt.Errorf("closing response body: %w", cerr)
	}
}

// builds an http.Request with Origin: hister:// set for CSRF bypass.
func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Origin", "hister://")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.accessToken != "" {
		req.Header.Set("X-Access-Token", c.accessToken)
	}
	if c.targetUserID != nil {
		req.Header.Set(targetUserIDHeader, strconv.FormatUint(uint64(*c.targetUserID), 10))
	}
	return req, nil
}

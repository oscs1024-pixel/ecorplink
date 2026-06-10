package corplink

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"ecorplink/internal/config"
)

const (
	matchURL  = "https://corplink.volcengine.cn/api/match"
	osQuery   = "os=Android&os_version=2"
	userAgent = "CorpLink/201000 (GooglePixel; Android 10; en)"
)

type apiResp[T any] struct {
	Code    int    `json:"code"`
	Action  string `json:"action,omitempty"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// Client is an HTTP client for the corplink API.
type Client struct {
	session          *Session
	mu               sync.RWMutex
	httpClient       *http.Client
	debugBody        bool
	matchURLOverride string // non-empty overrides the default matchURL (used in tests)
}

// NewClient creates a Client using the session's cookie jar.
func NewClient(session *Session) *Client {
	return NewClientWithConfig(session, config.DefaultConfig().Corplink)
}

// NewClientWithConfig creates a Client using the session's cookie jar and
// Corplink-specific transport/logging settings.
func NewClientWithConfig(session *Session, cfg config.CorplinkConfig) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}, //nolint:gosec // configurable for self-signed deployments
	}
	return &Client{
		session: session,
		httpClient: &http.Client{
			Jar:       session.Jar(),
			Transport: tr,
			Timeout:   20 * time.Second,
		},
		debugBody: cfg.DebugHTTPBody,
	}
}

func (c *Client) Configure(cfg config.CorplinkConfig) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}, //nolint:gosec // configurable for self-signed deployments
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.httpClient = &http.Client{
		Jar:       c.session.Jar(),
		Transport: tr,
		Timeout:   20 * time.Second,
	}
	c.debugBody = cfg.DebugHTTPBody
}

func (c *Client) post(ctx context.Context, rawURL string, body, out any) error {
	return c.do(ctx, http.MethodPost, rawURL, body, out)
}

func (c *Client) get(ctx context.Context, rawURL string, out any) error {
	return c.do(ctx, http.MethodGet, rawURL, nil, out)
}

func (c *Client) do(ctx context.Context, method, rawURL string, body, out any) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	c.mu.RLock()
	httpClient := c.httpClient
	debugBody := c.debugBody
	c.mu.RUnlock()
	log.Printf("[corplink] → %s %s", method, rawURL)
	if debugBody && len(bodyBytes) > 0 {
		log.Printf("[corplink] → body=%s", redactHTTPLogBody(bodyBytes, 512))
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	c.session.mu.Lock()
	serverStr := c.session.Server
	c.session.mu.Unlock()

	if serverStr != "" {
		if serverURL, err2 := url.Parse(serverStr); err2 == nil {
			reqURL, _ := url.Parse(rawURL)

			// Always inject all session cookies manually as a Cookie header.
			// This bypasses Go's cookiejar domain-matching so VPN node requests
			// (different host/IP) get the same cookies as the main server.
			cookies := c.session.jar.Cookies(serverURL)
			if len(cookies) > 0 {
				var parts []string
				for _, ck := range cookies {
					parts = append(parts, ck.Name+"="+ck.Value)
					if ck.Name == "csrf-token" {
						req.Header.Set("X-Csrf-Token", ck.Value)
					}
				}
				req.Header.Set("Cookie", strings.Join(parts, "; "))
				if reqURL != nil && reqURL.Host != serverURL.Host {
					log.Printf("[corplink] injected %d cookies for cross-host %s", len(parts), reqURL.Host)
				}
			}
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		log.Printf("[corplink] ← ERROR %s: %v", rawURL, err)
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	log.Printf("[corplink] ← %d %s", resp.StatusCode, rawURL)
	if debugBody && len(respBody) > 0 {
		log.Printf("[corplink] ← body=%s", redactHTTPLogBody(respBody, 1024))
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(out); err != nil {
		return err
	}
	// Detect session expiry from API response envelope before the caller checks.
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.NewDecoder(bytes.NewReader(respBody)).Decode(&envelope) == nil {
		if envelope.Code == 101 || envelope.Code == 10220002 {
			return fmt.Errorf("%w: %s", ErrSessionExpired, envelope.Message)
		}
	}
	return nil
}

func truncate(b []byte, max int) string {
	s := string(b)
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func (c *Client) httpBodyLoggingEnabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.debugBody
}

func redactHTTPLogBody(body []byte, max int) []byte {
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []byte("<redacted non-json body>")
	}
	redactJSONValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return []byte("<redacted body>")
	}
	if len(out) > max {
		return []byte(truncate(out, max))
	}
	return out
}

func redactJSONValue(v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if sensitiveHTTPLogKey(k) {
				delete(x, k)
				x["redacted"] = "[masked]"
				continue
			}
			redactJSONValue(child)
		}
	case []any:
		for _, child := range x {
			redactJSONValue(child)
		}
	}
}

func sensitiveHTTPLogKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	key = strings.ReplaceAll(key, "_", "-")
	switch key {
	case "authorization", "code", "cookie", "cookies", "csrf-token", "jwt", "login-url", "password", "private-key", "public-key", "qr-token", "server-public-key", "set-cookie", "token", "totp-secret", "user-name", "vpn-ip":
		return true
	default:
		return strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "cookie")
	}
}

// ErrSessionExpired is returned when the server reports a session expiry (code 101 or 10220002).
var ErrSessionExpired = fmt.Errorf("session expired")

// IsSessionExpired reports whether err is (or wraps) a session-expiry error.
func IsSessionExpired(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrSessionExpired.Error())
}

// CopySessionCookiesToURL copies authentication cookies from the main server
// to the given VPN node URL so that node API calls (ping, conn) are authorized.
// Cookies are stripped of their Domain attribute before copying so that
// Go's cookiejar accepts them for a different host (VPN node IP).
func (c *Client) CopySessionCookiesToURL(nodeBaseURL string) {
	c.session.mu.Lock()
	serverStr := c.session.Server
	c.session.mu.Unlock()
	if serverStr == "" {
		return
	}
	serverURL, err := url.Parse(serverStr)
	if err != nil {
		return
	}
	nodeURL, err := url.Parse(nodeBaseURL)
	if err != nil {
		return
	}
	cookies := c.session.jar.Cookies(serverURL)
	if len(cookies) == 0 {
		return
	}
	// Strip Domain so cookiejar accepts them for the node's IP host.
	stripped := make([]*http.Cookie, len(cookies))
	for i, ck := range cookies {
		stripped[i] = &http.Cookie{
			Name:  ck.Name,
			Value: ck.Value,
			Path:  "/",
		}
	}
	c.session.jar.SetCookies(nodeURL, stripped)
	log.Printf("[corplink] copied %d cookies from %s → %s", len(stripped), serverURL.Host, nodeURL.Host)
}

func (c *Client) apiURL(path string) string {
	c.session.mu.Lock()
	server := c.session.Server
	c.session.mu.Unlock()
	return server + path + "?" + osQuery
}

// DiscoverCompany resolves a company code to a server URL.
func (c *Client) DiscoverCompany(ctx context.Context, code string) error {
	type matchReq struct {
		Code string `json:"code"`
	}
	type companyData struct {
		Domain string `json:"domain"`
	}
	target := matchURL
	if c.matchURLOverride != "" {
		target = c.matchURLOverride
	}
	var resp apiResp[companyData]
	if err := c.post(ctx, target, matchReq{Code: code}, &resp); err != nil {
		return err
	}
	if resp.Code != 0 {
		return fmt.Errorf("discover company: %s", resp.Message)
	}
	if resp.Data.Domain == "" {
		return fmt.Errorf("discover company: empty domain in response")
	}
	c.session.mu.Lock()
	c.session.Server = resp.Data.Domain
	c.session.CompanyName = code
	c.session.rebuildJar()
	c.session.mu.Unlock()
	c.httpClient.Jar = c.session.Jar()
	return nil
}

// LoginMethods returns available login method identifiers.
// Returns: "email", "mobile", "lark" depending on server config.
func (c *Client) LoginMethods(ctx context.Context) ([]string, error) {
	type loginSetting struct {
		LoginOrders         []string `json:"login_orders"`
		LoginAccount        []string `json:"login_account"`
		ScanCodeLoginEnable bool     `json:"scan_code_login_enable"`
		ScanCodeTps         []string `json:"scan_code_tps"`
	}
	var resp apiResp[loginSetting]
	if err := c.get(ctx, c.apiURL("/api/login/setting"), &resp); err != nil {
		return nil, err
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("login setting: %s", resp.Message)
	}

	d := resp.Data
	var methods []string
	seen := map[string]bool{}

	add := func(m string) {
		if !seen[m] {
			seen[m] = true
			methods = append(methods, m)
		}
	}

	for _, order := range d.LoginOrders {
		switch order {
		case "feilian", "feilian_v1":
			for _, acc := range d.LoginAccount {
				switch acc {
				case "email":
					add("email")
				case "mobile":
					add("mobile")
				}
			}
		case "lark":
			if d.ScanCodeLoginEnable {
				add("lark")
			}
		}
	}

	// Fallback: scan_code_tps
	if d.ScanCodeLoginEnable {
		for _, tps := range d.ScanCodeTps {
			if tps == "lark" {
				add("lark")
			}
		}
	}

	return methods, nil
}

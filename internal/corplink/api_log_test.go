package corplink

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"

	"ecorplink/internal/config"
)

func TestRedactHTTPLogBodyMasksSensitiveFields(t *testing.T) {
	body := []byte(`{"user_name":"alice@example.com","code":"123456","token":"qr-token","totp_secret":"totp","nested":{"cookie":"secret-cookie"},"safe":"ok"}`)
	got := redactHTTPLogBody(body, 2048)
	text := string(got)
	for _, secret := range []string{"alice@example.com", "123456", "qr-token", "totp", "secret-cookie"} {
		if strings.Contains(text, secret) {
			t.Fatalf("redacted body still contains %q: %s", secret, text)
		}
	}
	if !strings.Contains(text, `"safe":"ok"`) {
		t.Fatalf("redacted body lost non-sensitive field: %s", text)
	}
}

func TestClientTLSAndBodyLoggingDefaults(t *testing.T) {
	cl := NewClient(LoadSession(t.TempDir() + "/session.json"))
	tr, ok := cl.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", cl.httpClient.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("default client should skip TLS verification")
	}
	if cl.httpBodyLoggingEnabled() {
		t.Fatal("HTTP body logging should be disabled by default")
	}
}

func TestClientTLSAndBodyLoggingFollowConfig(t *testing.T) {
	cl := NewClientWithConfig(LoadSession(t.TempDir()+"/session.json"), config.CorplinkConfig{
		InsecureSkipVerify: false,
		DebugHTTPBody:      true,
	})
	tr, ok := cl.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", cl.httpClient.Transport)
	}
	if tr.TLSClientConfig != nil && tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("insecure_skip_verify=false should enable TLS verification")
	}
	if !cl.httpBodyLoggingEnabled() {
		t.Fatal("debug_http_body=true should enable HTTP body logging")
	}
}

func TestClientSetDialContextPreservedAcrossConfigure(t *testing.T) {
	cl := NewClientWithConfig(LoadSession(t.TempDir()+"/session.json"), config.CorplinkConfig{
		InsecureSkipVerify: false,
	})
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, context.Canceled
	}

	cl.SetDialContext(dial)
	cl.Configure(config.CorplinkConfig{InsecureSkipVerify: true})

	tr, ok := cl.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", cl.httpClient.Transport)
	}
	if tr.DialContext == nil {
		t.Fatal("custom DialContext was not preserved")
	}
	if _, err := tr.DialContext(context.Background(), "tcp", "example.test:443"); err != context.Canceled {
		t.Fatalf("DialContext error = %v, want context.Canceled", err)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("Configure should still apply TLS settings")
	}
}

func TestRedactURLAndNetworkErrorForLog(t *testing.T) {
	rawURL := "https://vpn.example.invalid:443/api/vpn/list?os=Android&os_version=2"
	gotURL := redactURLForLog(rawURL)
	if strings.Contains(gotURL, "vpn.example.invalid") {
		t.Fatalf("redacted URL still contains host: %s", gotURL)
	}
	if !strings.Contains(gotURL, "/api/vpn/list") {
		t.Fatalf("redacted URL lost path: %s", gotURL)
	}

	errText := redactErrorForLog(rawURL, errors.New(`Get "`+rawURL+`": dial tcp 203.0.113.10:443: connect: can't assign requested address`))
	for _, secret := range []string{"vpn.example.invalid", "203.0.113.10"} {
		if strings.Contains(errText, secret) {
			t.Fatalf("redacted error still contains %q: %s", secret, errText)
		}
	}
}

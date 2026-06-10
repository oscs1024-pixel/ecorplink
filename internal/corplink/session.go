package corplink

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// Session holds persistent auth state.
type Session struct {
	CompanyName string          `json:"company_name"`
	Server      string          `json:"server"`
	DeviceID    string          `json:"device_id"`
	DeviceName  string          `json:"device_name"`
	TOTPSecret  string          `json:"totp_secret,omitempty"`
	Cookies     []*SerialCookie `json:"cookies,omitempty"`

	path string
	mu   sync.Mutex
	jar  http.CookieJar
}

// SerialCookie is a JSON-serializable http.Cookie.
type SerialCookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

// LoadSession loads a session from path, or returns a new empty session.
func LoadSession(path string) *Session {
	s := &Session{path: path}
	data, err := os.ReadFile(path)
	if err == nil {
		json.Unmarshal(data, s) //nolint:errcheck
	}
	if s.DeviceID == "" {
		s.DeviceID = newUUID()
	}
	if s.DeviceName == "" {
		s.DeviceName = machineName()
	}
	s.rebuildJar()
	return s
}

// Save persists the session to disk.
func (s *Session) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncCookiesFromJar()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600)
}

// Jar returns the cookie jar for HTTP requests.
func (s *Session) Jar() http.CookieJar {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.jar
}

// IsAuthenticated returns true if a server URL and cookies are present.
func (s *Session) IsAuthenticated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Server != "" && len(s.Cookies) > 0
}

func (s *Session) rebuildJar() {
	jar, _ := cookiejar.New(nil)
	if s.Server != "" {
		serverURL, err := url.Parse(s.Server)
		if err == nil {
			// Always inject device_id and device_name like corplink-rs does.
			// These are required for session identification by the server.
			var cookies []*http.Cookie
			if s.DeviceID != "" {
				cookies = append(cookies, &http.Cookie{Name: "device_id", Value: s.DeviceID, Path: "/"})
			}
			if s.DeviceName != "" {
				cookies = append(cookies, &http.Cookie{Name: "device_name", Value: s.DeviceName, Path: "/"})
			}
			// Append saved session cookies.
			for _, sc := range s.Cookies {
				cookies = append(cookies, &http.Cookie{
					Name: sc.Name, Value: sc.Value,
					Domain: sc.Domain, Path: sc.Path,
				})
			}
			jar.SetCookies(serverURL, cookies)
		}
	}
	s.jar = jar
}

func (s *Session) syncCookiesFromJar() {
	if s.Server == "" || s.jar == nil {
		return
	}
	serverURL, err := url.Parse(s.Server)
	if err != nil {
		return
	}
	s.Cookies = nil
	for _, c := range s.jar.Cookies(serverURL) {
		s.Cookies = append(s.Cookies, &SerialCookie{
			Name: c.Name, Value: c.Value,
			Domain: c.Domain, Path: c.Path,
		})
	}
}

func newUUID() string {
	b := make([]byte, 16)
	if f, err := os.Open("/dev/urandom"); err == nil {
		f.Read(b) //nolint:errcheck
		f.Close()
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func machineName() string {
	host, err := os.Hostname()
	if err != nil {
		return "macOS-device"
	}
	return "macOS-" + host
}

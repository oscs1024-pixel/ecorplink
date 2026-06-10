package gui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunConnectivityTestsTreatsHTTPAuthAsReachable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	svc := NewService(Options{Runner: &fakeRunner{}})
	report := svc.RunConnectivityTests(TestRequest{
		TimeoutMillis: 1000,
		HTTPClient:    server.Client(),
		Items: []TestTarget{{
			Name:           "api",
			URL:            server.URL,
			ExpectedPolicy: "DEFAULT",
		}},
	})
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	got := report.Results[0]
	if !got.Reachable || got.HTTPStatus != 403 || got.DurationMillis <= 0 {
		t.Fatalf("result = %+v", got)
	}
}

func TestRunConnectivityTestsMarksTimeoutUnreachable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	svc := NewService(Options{Runner: &fakeRunner{}})
	report := svc.RunConnectivityTests(TestRequest{
		TimeoutMillis: 20,
		Items: []TestTarget{{
			Name: "slow",
			URL:  server.URL,
		}},
	})
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	if report.Results[0].Reachable || report.Results[0].Error == "" {
		t.Fatalf("result = %+v", report.Results[0])
	}
}

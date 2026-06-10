package gui

import (
	"context"
	"net/http"
	"sync"
	"time"
)

func (s *Service) RunConnectivityTests(req TestRequest) TestReport {
	timeout := time.Duration(req.TimeoutMillis) * time.Millisecond
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	targets := req.Items
	if len(targets) == 0 {
		targets = defaultTestTargets()
	}
	client := req.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	} else {
		copyClient := *client
		copyClient.Timeout = timeout
		client = &copyClient
	}

	// Run all tests concurrently; collect in original order.
	results := make([]TestResult, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(idx int, t TestTarget) {
			defer wg.Done()
			results[idx] = runHTTPTest(client, timeout, t)
		}(i, target)
	}
	wg.Wait()
	return TestReport{OK: true, Results: results}
}

// RunSingleTest runs one connectivity test. Used by the frontend to
// trigger individual tests in parallel and show results as they arrive.
func (s *Service) RunSingleTest(target TestTarget) TestResult {
	return runHTTPTest(&http.Client{Timeout: 8 * time.Second}, 8*time.Second, target)
}

func runHTTPTest(client *http.Client, timeout time.Duration, target TestTarget) TestResult {
	start := time.Now()
	result := TestResult{
		Name: target.Name,
		URL:  target.URL,
	}
	if target.URL == "" {
		result.Error = "empty URL"
		return result
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	resp, err := client.Do(req)
	result.DurationMillis = time.Since(start).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	result.Reachable = isReachableStatus(resp.StatusCode)
	return result
}

func isReachableStatus(code int) bool {
	switch code {
	case 200, 204, 301, 302, 401, 403, 404:
		return true
	default:
		return false
	}
}

func defaultTestTargets() []TestTarget {
	return []TestTarget{
		{Name: "Baidu", URL: "https://www.baidu.com"},
		{Name: "Bing", URL: "https://www.bing.com"},
		{Name: "QQ", URL: "https://www.qq.com"},
		{Name: "Google", URL: "https://www.google.com/generate_204"},
		{Name: "GitHub", URL: "https://github.com"},
		{Name: "YouTube", URL: "https://www.youtube.com/generate_204"},
		{Name: "X", URL: "https://x.com"},
	}
}

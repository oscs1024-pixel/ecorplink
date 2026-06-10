package gui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	calls   []RunRequest
	next    RunResult
	results []RunResult
	after   func(req RunRequest)
}

func (r *fakeRunner) Run(ctx context.Context, req RunRequest) RunResult {
	r.calls = append(r.calls, req)
	if r.after != nil {
		defer r.after(req)
	}
	if len(r.results) > 0 {
		res := r.results[0]
		r.results = r.results[1:]
		return res
	}
	if r.next == (RunResult{}) {
		return RunResult{OK: true}
	}
	return r.next
}

func TestDaemonCommandsUseConfiguredBinaryAndConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runner := &fakeRunner{next: RunResult{OK: true, Stdout: "daemon started (pid 1234)\n"}}
	svc := NewService(Options{
		DaemonPath: "/tmp/ecorplink-daemon",
		ConfigPath: "/tmp/config.json",
		Runner:     runner,
	})
	runner.after = func(req RunRequest) {
		if strings.Contains(strings.Join(req.Args, " "), "/tmp/config.json") {
			_ = os.MkdirAll(filepath.Dir(svc.pidPath), 0755)
			_ = os.WriteFile(svc.pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
		}
	}

	if res := svc.StartDaemon(""); !res.OK {
		t.Fatalf("StartDaemon OK = false: %+v", res)
	}
	if res := svc.StopDaemon(); !res.OK {
		t.Fatalf("StopDaemon OK = false: %+v", res)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("runner calls = %d, want 2", len(runner.calls))
	}
	if runtime.GOOS == "darwin" {
		if runner.calls[0].Path != "osascript" {
			t.Fatalf("start path = %q, want osascript", runner.calls[0].Path)
		}
		if got := strings.Join(runner.calls[0].Args, " "); !strings.Contains(got, "/tmp/ecorplink-daemon") || strings.Contains(got, " start ") || !strings.Contains(got, "/tmp/config.json") || !strings.Contains(got, "administrator privileges") {
			t.Fatalf("start args = %q, want privileged daemon helper", got)
		}
		if runner.calls[1].Path != "osascript" {
			t.Fatalf("stop path = %q, want osascript", runner.calls[1].Path)
		}
		if got := strings.Join(runner.calls[1].Args, " "); !strings.Contains(got, "/tmp/ecorplink-daemon") || !strings.Contains(got, "stop") || !strings.Contains(got, "administrator privileges") {
			t.Fatalf("stop args = %q, want privileged daemon stop", got)
		}
		return
	}
	if got := strings.Join(runner.calls[0].Args, " "); !strings.Contains(got, "-c /tmp/config.json --pid-file ") || strings.Contains(got, "start") {
		t.Fatalf("start args = %q", got)
	}
	if got := strings.Join(runner.calls[1].Args, " "); !strings.HasPrefix(got, "stop --pid-file ") {
		t.Fatalf("stop args = %q", got)
	}
	if runner.calls[0].Path != "/tmp/ecorplink-daemon" {
		t.Fatalf("daemon path = %q", runner.calls[0].Path)
	}
}

func TestNewServiceDefaultsToEcorplinkHomePaths(t *testing.T) {
	svc := NewService(Options{Runner: &fakeRunner{}})
	if !strings.Contains(svc.configPath, filepath.Join(".ecorplink", "config.json")) {
		t.Fatalf("config path = %q, want ~/.ecorplink/config.json", svc.configPath)
	}
	if !strings.Contains(svc.logPath, filepath.Join(".ecorplink", "ecorplink.log")) {
		t.Fatalf("log path = %q, want ~/.ecorplink/ecorplink.log", svc.logPath)
	}
	if !strings.Contains(svc.daemonPath, filepath.Join(".ecorplink", "bin", "ecorplink-daemon")) {
		t.Fatalf("daemon path = %q, want ~/.ecorplink/bin/ecorplink-daemon", svc.daemonPath)
	}
}

func TestCommandResultPrefersCommandOutputOverGenericExitStatus(t *testing.T) {
	res := commandResult(RunResult{
		OK:       false,
		Stderr:   "read config /missing/config.json: no such file or directory\n",
		Error:    "exit status 1",
		ExitCode: 1,
	})

	if res.OK {
		t.Fatal("commandResult OK = true, want false")
	}
	if res.Summary != "read config /missing/config.json: no such file or directory" {
		t.Fatalf("summary = %q", res.Summary)
	}
	if !strings.Contains(res.Details, "stderr:") || !strings.Contains(res.Details, "exit code: 1") || !strings.Contains(res.Details, "error: exit status 1") {
		t.Fatalf("details = %q, want stderr, exit code, and exec error", res.Details)
	}
}

func TestStartDaemonReturnsSuccessWhenStatusIsAlreadyRunning(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runner := &fakeRunner{}
	svc := NewService(Options{DaemonPath: "/tmp/ecorplink-daemon", ConfigPath: "/tmp/config.json", Runner: runner})
	if err := os.MkdirAll(filepath.Dir(svc.pidPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	res := svc.StartDaemon("")
	if !res.OK {
		t.Fatalf("StartDaemon = %+v, want OK", res)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want pidfile status only", len(runner.calls))
	}
	if !strings.Contains(res.Summary, "daemon running") {
		t.Fatalf("summary = %q, want daemon running", res.Summary)
	}
}

func TestStartDaemonTreatsAlreadyRunningCommandErrorAsRunningStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	runner := &fakeRunner{
		results: []RunResult{
			{OK: false, Stderr: "daemon already running (pid 1234)\n", Error: "exit status 1", ExitCode: 1},
		},
	}
	svc := NewService(Options{DaemonPath: "/tmp/ecorplink-daemon", ConfigPath: "/tmp/config.json", Runner: runner})
	runner.after = func(req RunRequest) {
		_ = os.MkdirAll(filepath.Dir(svc.pidPath), 0755)
		_ = os.WriteFile(svc.pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
	}

	res := svc.StartDaemon("")
	if !res.OK {
		t.Fatalf("StartDaemon = %+v, want OK", res)
	}
	if !strings.Contains(res.Summary, "daemon running") {
		t.Fatalf("summary = %q, want daemon running", res.Summary)
	}
}

func TestStartDaemonFailsWhenDaemonNeverBecomesRunning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runner := &fakeRunner{
		results: []RunResult{
			{OK: true, Stdout: "daemon started (pid 1234)\n"},
		},
	}
	svc := NewService(Options{DaemonPath: "/tmp/ecorplink-daemon", ConfigPath: "/tmp/config.json", Runner: runner})

	res := svc.StartDaemon("")
	if res.OK {
		t.Fatalf("StartDaemon = %+v, want failure", res)
	}
	if !strings.Contains(res.Summary, "did not become running") {
		t.Fatalf("summary = %q, want did not become running", res.Summary)
	}
}

func TestGetDaemonStatusReadsPidFileDirectly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	runner := &fakeRunner{next: RunResult{Stdout: "daemon not running\n"}}
	svc := NewService(Options{DaemonPath: "/tmp/ecorplink-daemon", Runner: runner})
	if err := os.MkdirAll(filepath.Dir(svc.pidPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	status := svc.GetDaemonStatus()
	if status.State != "running" || status.PID != os.Getpid() {
		t.Fatalf("status = %+v, want current process running", status)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %d, want pidfile status without CLI", len(runner.calls))
	}
}

func TestLoadSaveAndValidateConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"rules":[{"enabled":true,"type":"DOMAIN","value":"github.com","policy":"DIRECT"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Options{ConfigPath: path, Runner: &fakeRunner{}})

	doc := svc.LoadConfig("")
	if !doc.OK || len(doc.Config.Rules) != 1 {
		t.Fatalf("LoadConfig = %+v", doc)
	}
	doc.Config.Rules[0].Comment = "edited"
	res := svc.SaveConfig("", doc.Config)
	if !res.OK {
		t.Fatalf("SaveConfig = %+v", res)
	}
	reloaded := svc.LoadConfig("")
	if reloaded.Config.Rules[0].Comment != "edited" {
		t.Fatalf("comment after reload = %q", reloaded.Config.Rules[0].Comment)
	}
}

func TestLoadConfigCreatesDefaultTemplateWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	svc := NewService(Options{ConfigPath: path, Runner: &fakeRunner{}})

	doc := svc.LoadConfig("")
	if !doc.OK || doc.Config == nil {
		t.Fatalf("LoadConfig = %+v", doc)
	}
	if len(doc.Config.Rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(doc.Config.Rules))
	}
	rule := doc.Config.Rules[0]
	if !rule.Enabled || rule.Type != "GEOIP" || rule.Value != "CN" || rule.Policy != "DIRECT" {
		t.Fatalf("default rule = %+v, want GEOIP CN DIRECT", rule)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("default config was not written: %v", err)
	}
}

func TestReadLogTailsBoundedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ecorplink.log")
	data := strings.Join([]string{"one", "two", "three", "four"}, "\n")
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Options{LogPath: path, Runner: &fakeRunner{}})
	chunk := svc.ReadLog(LogRequest{Lines: 2})
	if !chunk.OK {
		t.Fatalf("ReadLog = %+v", chunk)
	}
	if strings.TrimSpace(chunk.Text) != "three\nfour" {
		t.Fatalf("tail = %q", chunk.Text)
	}
}

func TestExecRunnerHonorsTimeout(t *testing.T) {
	runner := ExecRunner{Timeout: 20 * time.Millisecond}
	res := runner.Run(context.Background(), RunRequest{
		Path: "sh",
		Args: []string{"-c", "sleep 1"},
	})
	if res.OK || !res.TimedOut {
		t.Fatalf("Run = %+v, want timeout", res)
	}
}

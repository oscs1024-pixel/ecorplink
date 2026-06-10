package gui

import (
	"strings"
	"testing"
)

func TestLaunchDaemonPlistUsesDirectDaemonProgramArguments(t *testing.T) {
	plist := LaunchDaemonPlist(LaunchServiceRequest{
		Label:      "com.ecorplink.daemon",
		BinaryPath: "/opt/ecorplink/ecorplink-daemon",
		ConfigPath: "/opt/ecorplink/config.json",
		WorkDir:    "/opt/ecorplink",
	})

	for _, want := range []string{
		"<string>/opt/ecorplink/ecorplink-daemon</string>",
		"<string>-c</string>",
		"<string>/opt/ecorplink/config.json</string>",
		"<key>KeepAlive</key>",
		"<true/>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "<string>start</string>") {
		t.Fatalf("plist should not use a start subcommand:\n%s", plist)
	}
}

func TestInstallLaunchServiceUsesRunner(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewService(Options{Runner: runner})
	res := svc.InstallLaunchService(LaunchServiceRequest{
		Label:      "com.ecorplink.daemon",
		BinaryPath: "/opt/ecorplink/ecorplink-daemon",
		ConfigPath: "/opt/ecorplink/config.json",
		WorkDir:    "/opt/ecorplink",
	})
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1; result=%+v", len(runner.calls), res)
	}
	if runner.calls[0].Path == "" {
		t.Fatalf("runner path is empty")
	}
}

func TestUninstallLaunchServiceRemovesDarwinPlist(t *testing.T) {
	runner := &fakeRunner{}
	svc := NewService(Options{Runner: runner})
	res := svc.uninstallDarwinLaunchService("com.ecorplink.daemon")
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %d, want 1; result=%+v", len(runner.calls), res)
	}
	call := runner.calls[0]
	if call.Path != "osascript" {
		t.Fatalf("uninstall path = %q, want osascript", call.Path)
	}
	args := strings.Join(call.Args, " ")
	for _, want := range []string{
		"administrator privileges",
		"launchctl bootout system/com.ecorplink.daemon",
		"rm -f /Library/LaunchDaemons/com.ecorplink.daemon.plist",
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("uninstall args missing %q:\n%s", want, args)
		}
	}
}

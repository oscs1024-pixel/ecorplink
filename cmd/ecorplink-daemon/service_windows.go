//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

func isWindowsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isSvc
}

// elog is the Windows event log for service mode.
var elog debug.Log

// ecorplinkService implements svc.Handler.
type ecorplinkService struct{}

func (s *ecorplinkService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				elog.Info(1, "service stopping")
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						elog.Error(1, fmt.Sprintf("stop error: %v", err))
						return true, 1
					}
				case <-time.After(15 * time.Second):
					elog.Error(1, "service stop timed out")
					return true, 1
				}
				return false, 0
			default:
				elog.Error(1, fmt.Sprintf("unexpected control request #%d", c))
			}
		case err := <-errCh:
			if err != nil {
				elog.Error(1, fmt.Sprintf("run error: %v", err))
				return true, 1
			}
			return false, 0
		}
	}
}

func runService() error {
	var err error
	elog, err = eventlog.Open("ecorplink")
	if err != nil {
		return err
	}
	defer elog.Close()

	elog.Info(1, "starting service")
	err = svc.Run("ecorplink", &ecorplinkService{})
	if err != nil {
		elog.Error(1, fmt.Sprintf("service failed: %v", err))
		return err
	}
	elog.Info(1, "service stopped")
	return nil
}

// installService registers ecorplink as a Windows service.
func installService(configPath, pidFile string) error {
	exepath, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService("ecorplink")
	if err == nil {
		s.Close()
		return fmt.Errorf("service ecorplink already exists")
	}

	args := []string{}
	if configPath != "" {
		args = append(args, "-c", configPath)
	}
	if pidFile != "" {
		args = append(args, "--pid-file", pidFile)
	}
	s, err = m.CreateService("ecorplink", exepath, mgr.Config{
		DisplayName: "ECorpLink",
		Description: "ECorpLink VPN daemon",
		StartType:   mgr.StartAutomatic,
	}, args...)
	if err != nil {
		return err
	}
	defer s.Close()

	err = eventlog.InstallAsEventCreate("ecorplink", eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil {
		s.Delete()
		return fmt.Errorf("setup event log: %v", err)
	}

	fmt.Println("service installed")
	return nil
}

// uninstallService removes the Windows service.
func uninstallService() error {
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	s, err := m.OpenService("ecorplink")
	if err != nil {
		return fmt.Errorf("service ecorplink does not exist")
	}
	defer s.Close()

	s.Control(svc.Stop)
	s.Delete()
	eventlog.Remove("ecorplink")

	fmt.Println("service removed")
	return nil
}

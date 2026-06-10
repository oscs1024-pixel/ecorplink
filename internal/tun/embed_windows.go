//go:build windows

package tun

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed wintun/bin/amd64/wintun.dll
var wintunAMD64 []byte

//go:embed wintun/bin/x86/wintun.dll
var wintunX86 []byte

//go:embed wintun/bin/arm64/wintun.dll
var wintunARM64 []byte

//go:embed wintun/bin/arm/wintun.dll
var wintunARM []byte

func init() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	dll := wintunDLLForArch(runtime.GOARCH)
	if len(dll) == 0 {
		return
	}
	dir := filepath.Dir(exePath)
	dllPath := filepath.Join(dir, "wintun.dll")
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		_ = os.WriteFile(dllPath, dll, 0644)
	}
}

func wintunDLLForArch(goarch string) []byte {
	switch goarch {
	case "amd64":
		return wintunAMD64
	case "386":
		return wintunX86
	case "arm64":
		return wintunARM64
	case "arm":
		return wintunARM
	default:
		return nil
	}
}

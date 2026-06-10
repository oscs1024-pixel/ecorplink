//go:build windows

package tun

import "testing"

func TestWintunDLLForArch(t *testing.T) {
	tests := map[string][]byte{
		"amd64": wintunAMD64,
		"386":   wintunX86,
		"arm64": wintunARM64,
		"arm":   wintunARM,
	}
	for arch, want := range tests {
		got := wintunDLLForArch(arch)
		if len(got) == 0 {
			t.Fatalf("wintunDLLForArch(%q) returned empty data", arch)
		}
		if len(got) != len(want) {
			t.Fatalf("wintunDLLForArch(%q) size = %d, want %d", arch, len(got), len(want))
		}
	}
	if got := wintunDLLForArch("mips"); got != nil {
		t.Fatalf("unsupported arch returned %d bytes", len(got))
	}
}

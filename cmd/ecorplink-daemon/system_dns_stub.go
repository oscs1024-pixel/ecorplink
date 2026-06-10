//go:build !darwin && !linux && !windows

package main

type systemDNSState struct{}

func systemDNSAddr() string { return "" }

func systemDNSOriginalUpstream() string { return "" }

func prepareSystemDNSListener() error { return nil }

func cleanupSystemDNSListener() {}

func setupSystemDNS(_, _ string) (*systemDNSState, error) { return nil, nil }

func cleanupSystemDNS() {}

func flushDNSCache() {}

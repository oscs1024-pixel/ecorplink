package main

import "ecorplink/internal/config"

func shouldSetupSystemDNS(cfg *config.Config) bool {
	return cfg != nil && cfg.DNS.SystemHijack
}

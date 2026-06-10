package geoip

import _ "embed"

//go:embed Country.mmdb
var embeddedDB []byte

package geoip

import (
	"fmt"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// DB wraps a MaxMind GeoIP2 Country database reader.
type DB struct {
	reader *maxminddb.Reader
}

// Open loads a GeoIP2 Country database.
// path="" uses the embedded default (which may be empty/stub).
// Returns a valid *DB even if no data is available (Country returns "" for all IPs).
func Open(path string) (*DB, error) {
	if path == "" {
		return openEmbedded()
	}
	r, err := maxminddb.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geoip: open %q: %w", path, err)
	}
	return &DB{reader: r}, nil
}

// Close releases resources held by the database.
func (db *DB) Close() error {
	if db.reader != nil {
		return db.reader.Close()
	}
	return nil
}

// Country returns the ISO 3166-1 alpha-2 country code for ip,
// or "" if unknown or unavailable.
func (db *DB) Country(ip net.IP) (string, error) {
	if db.reader == nil {
		return "", nil
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := db.reader.Lookup(ip, &record); err != nil {
		return "", fmt.Errorf("geoip: lookup %v: %w", ip, err)
	}
	return record.Country.ISOCode, nil
}

// IsChina reports whether ip is geolocated to China.
func (db *DB) IsChina(ip net.IP) bool {
	cc, err := db.Country(ip)
	if err != nil {
		return false
	}
	return cc == "CN"
}

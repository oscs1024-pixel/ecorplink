package geoip

import "github.com/oschwald/maxminddb-golang"

func openEmbedded() (*DB, error) {
	if len(embeddedDB) == 0 {
		return &DB{}, nil // no-op DB, Country() returns ""
	}
	r, err := maxminddb.FromBytes(embeddedDB)
	if err != nil {
		return nil, err
	}
	return &DB{reader: r}, nil
}

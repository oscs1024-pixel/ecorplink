package geoip

import (
	"net"
	"os"
	"testing"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

func buildTestDB(t *testing.T) string {
	t.Helper()
	writer, err := mmdbwriter.New(mmdbwriter.Options{DatabaseType: "GeoIP2-Country"})
	if err != nil {
		t.Fatal(err)
	}
	_, cn, _ := net.ParseCIDR("1.2.4.0/24")
	writer.Insert(cn, mmdbtype.Map{
		"country": mmdbtype.Map{"iso_code": mmdbtype.String("CN")},
	})
	f, _ := os.CreateTemp("", "test-*.mmdb")
	writer.WriteTo(f)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestCountry(t *testing.T) {
	path := buildTestDB(t)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cc, err := db.Country(net.ParseIP("1.2.4.1"))
	if err != nil {
		t.Fatal(err)
	}
	if cc != "CN" {
		t.Errorf("want CN, got %s", cc)
	}

	cc2, _ := db.Country(net.ParseIP("8.8.8.8"))
	if cc2 == "CN" {
		t.Error("8.8.8.8 should not be CN")
	}
}

func TestIsChina(t *testing.T) {
	path := buildTestDB(t)
	db, _ := Open(path)
	defer db.Close()
	if !db.IsChina(net.ParseIP("1.2.4.2")) {
		t.Error("1.2.4.2 should be CN")
	}
	if db.IsChina(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should not be CN")
	}
}

func TestEmbeddedDB(t *testing.T) {
	// Open with empty path uses the embedded database when it is present.
	db, err := Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cc, err := db.Country(net.ParseIP("1.2.4.1"))
	if err != nil {
		t.Fatalf("embedded DB should not error: %v", err)
	}
	if cc == "" {
		t.Error("embedded DB should return a country for 1.2.4.1")
	}
}

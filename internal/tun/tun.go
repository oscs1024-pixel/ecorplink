package tun

import (
	"fmt"
	"net"
	"os"

	"golang.zx2c4.com/wireguard/tun"
)

// Config holds TUN device configuration.
type Config struct {
	Name string
	IP   string
	Mask int
	MTU  int
}

// Device wraps a wireguard/tun device with lifecycle management.
type Device struct {
	dev    tun.Device
	config Config
}

func NewDevice(cfg Config) (*Device, error) {
	dev, err := tun.CreateTUN(cfg.Name, cfg.MTU)
	if err != nil {
		return nil, fmt.Errorf("create tun: %w", err)
	}
	d := &Device{dev: dev, config: cfg}
	if err := d.configureIP(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure tun ip: %w", err)
	}
	return d, nil
}

func (d *Device) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	return d.dev.Read(bufs, sizes, offset)
}

func (d *Device) Write(bufs [][]byte, offset int) (int, error) {
	return d.dev.Write(bufs, offset)
}

func (d *Device) Close() error {
	return d.dev.Close()
}

func (d *Device) Name() (string, error) {
	return d.dev.Name()
}

// PeerIP returns the peer IP for a point-to-point TUN interface.
// For a /30 subnet with local IP 172.30.77.1, peer is 172.30.77.2.
func (d *Device) PeerIP() string {
	ip := net.ParseIP(d.config.IP).To4()
	if ip == nil {
		return ""
	}
	peerIP := make(net.IP, len(ip))
	copy(peerIP, ip)
	peerIP[3]++
	return peerIP.String()
}

func (d *Device) MTU() (int, error) {
	return d.dev.MTU()
}

func (d *Device) BatchSize() int {
	return d.dev.BatchSize()
}

func (d *Device) File() *os.File {
	return d.dev.File()
}

func (d *Device) Events() <-chan tun.Event {
	return d.dev.Events()
}

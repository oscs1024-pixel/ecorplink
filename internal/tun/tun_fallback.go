//go:build !darwin && !linux && !windows

package tun

func (d *Device) configureIP() error {
	return nil
}

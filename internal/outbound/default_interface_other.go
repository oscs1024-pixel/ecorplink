//go:build !darwin

package outbound

import "net"

func defaultRouteInterface() (*net.Interface, error) {
	return nil, nil
}

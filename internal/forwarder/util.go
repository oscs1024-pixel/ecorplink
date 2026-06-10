package forwarder

import (
	"io"
	"net"
	"sync/atomic"
	"time"
)

func deadlineIn(secs int) time.Time {
	return time.Now().Add(time.Duration(secs) * time.Second)
}

type closeWriter interface {
	CloseWrite() error
}

func relay(a, b net.Conn, closed *atomic.Bool) {
	done := make(chan struct{}, 2)
	half := func(dst, src net.Conn) {
		defer func() { done <- struct{}{} }()
		io.Copy(dst, src) //nolint:errcheck
		if cw, ok := dst.(closeWriter); ok {
			cw.CloseWrite() //nolint:errcheck
		}
	}
	go half(b, a)
	go half(a, b)
	<-done
	<-done
}

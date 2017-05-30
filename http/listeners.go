package http

import (
	"net"
	"github.com/kavu/go_reuseport"
)

func CreateListener(proto string, laddr string, reusePort bool) (net.Listener, error) {
	if reusePort {
		return reuseport.Listen(proto, laddr)
	} else {
		return net.Listen(proto, laddr)
	}
}

package netpoll

import (
	"net"

	"github.com/libp2p/go-reuseport"
)

func ReusePortListen(proto, addr string) (net.Listener, error) {
	return reuseport.Listen(proto, addr)
}

func ReusePortListenPacket(proto, addr string) (net.PacketConn, error) {
	return reuseport.ListenPacket(proto, addr)
}

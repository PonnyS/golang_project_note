package netpoll

import (
	"golang.org/x/sys/unix"
)

func SetKeepAlive(fd, secs int) error {
	if err := unix.SetsockoptInt(fd, unix.SOL_SOCKET, 0x8, 1); err != nil {
		return err
	}
	switch err := unix.SetsockoptInt(fd, unix.IPPROTO_TCP, 0x101, secs); err {
	case nil, unix.ENOPROTOOPT:
	default:
		return err
	}
	return unix.SetsockoptInt(fd, unix.IPPROTO_TCP, unix.TCP_KEEPALIVE, secs)
}

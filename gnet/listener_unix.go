package gnet

import (
	"golang.org/x/sys/unix"
	"net"
	"os"
	"sync"
)

type listener struct {
	once sync.Once
	// 文件描述符
	f  *os.File
	fd int
	// TCP
	ln net.Listener
	// UDP
	pconn net.PacketConn
	// 地址的抽象接口
	lnaddr net.Addr
	// network: tcp
	// addr: 127.0.0.1
	addr, network string
}

// 1. 获取描述符
// 2. 设置非阻塞fd
func (ln *listener) renormalize() error {
	var err error
	switch netln := ln.ln.(type) {
	case nil:
		switch pconn := ln.pconn.(type) {
		case *net.UDPConn:
			ln.f, err = pconn.File()
		}
	case *net.TCPListener:
		ln.f, err = netln.File()
	case *net.UnixListener:
		ln.f, err = netln.File()
	}
	if err != nil {
		ln.close()
		return err
	}
	ln.fd = int(ln.f.Fd())
	return unix.SetNonblock(ln.fd, true)
}

func (ln *listener) close() {
	ln.once.Do(func() {
		if ln.f != nil {
			sniffErrorAndLog(ln.f.Close())
		}
		if ln.ln != nil {
			sniffErrorAndLog(ln.ln.Close())
		}
		if ln.pconn != nil {
			sniffErrorAndLog(ln.pconn.Close())
		}
		if ln.network == "unix" {
			sniffErrorAndLog(os.RemoveAll(ln.addr))
		}
	})
}

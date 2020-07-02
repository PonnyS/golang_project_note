package gnet

import (
	"net"
	"time"

	"golang.org/x/sys/unix"
	"golang_project_note/gnet/internal/netpoll"
)

type eventloop struct {
	svr    *server
	poller *netpoll.Poller
	// 在server.subEventLoopSet 中的下标
	idx    int
	codec  ICodec
	packet []byte
	// eventloop 中活跃的连接数
	connCount int32
	// fd -> conn
	connections  map[int]*conn
	eventHandler EventHandler
	// 负载均衡的再调整
	calibrateCallback func(*eventloop, int32)
}

func (el *eventloop) closeAllConns() {
	for _, c := range el.connections {
		_ = el.loopCloseConn(c, nil)
	}
}

func (el *eventloop) loopCloseConn(c *conn, err error) error {
	// 正常关闭，还有数据没发送给客户端
	if !c.outboundBuffer.IsEmpty() && err == nil {
		_ = el.loopWrite(c)
	}
	// 1. 删除fd上的注册事件
	// 2. 关闭fd
	err0, err1 := el.poller.Delete(c.fd), unix.Close(c.fd)
	if err0 == nil && err1 == nil {
		delete(el.connections, c.fd)
		// 负载均衡的再调整
		el.calibrateCallback(el, -1)
		switch el.eventHandler.OnClosed(c, err) {
		case Shutdown:
			return errServerShutdown
		}
		// 手动回收conn内存
		c.releaseTCP()
	} else {
		if err0 != nil {
			el.svr.logger.Printf("failed to delete fd:%d from poller, error:%v\n", c.fd, err0)
		}
		if err1 != nil {
			el.svr.logger.Printf("failed to close fd:%d, error:%v\n", c.fd, err1)
		}
	}
	return nil
}

func (el *eventloop) loopWrite(c *conn) error {
	el.eventHandler.PreWrite()

	head, tail := c.outboundBuffer.LazyReadAll()
	n, err := unix.Write(c.fd, head)
	if err != nil {
		if err == unix.EAGAIN {
			return nil
		}
		return el.loopCloseConn(c, err)
	}
	c.outboundBuffer.Shift(n)

	// 前提必须是head已经写完，才能写tail，不然数据会错乱
	if len(head) == n && tail != nil {
		n, err = unix.Write(c.fd, tail)
		if err != nil {
			if err == unix.EAGAIN {
				return nil
			}
			return el.loopCloseConn(c, err)
		}
		c.outboundBuffer.Shift(n)
	}

	// 数据都发送完了，fd设置可读事件（不需要监听可写事件了）
	if c.outboundBuffer.IsEmpty() {
		_ = el.poller.ModRead(c.fd)
	}
	return nil
}

func (el *eventloop) loopWake(c *conn) error {
	out, action := el.eventHandler.React(nil, c)
	if out != nil {
		frame, _ := el.codec.Encode(c, out)
		c.write(frame)
	}
	return el.handleAction(c, action)
}

func (el *eventloop) loopRun() {
	defer func() {
		el.closeAllConns()
		if el.idx == 0 && el.svr.opts.Ticker {
			close(el.svr.ticktock)
		}
		// TODO why one loop exit leads to server shutdown?
		el.svr.signalShutdown()
	}()

	if el.idx == 0 && el.svr.opts.Ticker {
		go el.loopTicker()
	}

	el.svr.logger.Printf("event-loop:%d exits with error: %v\n", el.idx, el.poller.Polling(el.handleEvent))
}

func (el *eventloop) loopTicker() {
	var (
		err    error
		deplay time.Duration
		open   bool
	)
	for {
		err = el.poller.Trigger(func() (err error) {
			deplay, action := el.eventHandler.Tick()
			el.svr.ticktock <- deplay
			switch action {
			case None:
			case Shutdown:
				err = errServerShutdown
			}
			return
		})
		if err != nil {
			el.svr.logger.Printf("failed to awake poller with error:%v, stopping ticker\n", err)
			break
		}
		if deplay, open = <-el.svr.ticktock; open {
			time.Sleep(deplay)
		} else {
			break
		}
	}
}

func (el *eventloop) handleAction(c *conn, action Action) error {
	switch action {
	case None:
		return nil
	case Close:
		return el.loopCloseConn(c, nil)
	case Shutdown:
		return errServerShutdown
	default:
		return nil
	}
}

func (el *eventloop) loopOpen(c *conn) error {
	c.opened = true
	c.localAddr = el.svr.ln.lnaddr
	c.remoteAddr = netpoll.SockaddrToTCPOrUnixAddr(c.sa)
	out, action := el.eventHandler.OnOpened(c)
	if el.svr.opts.TCPKeepAlive > 0 {
		if _, ok := el.svr.ln.ln.(*net.TCPListener); ok {
			_ = netpoll.SetKeepAlive(c.fd, int(el.svr.opts.TCPKeepAlive/time.Second))
		}
	}
	if out != nil {
		c.open(out)
	}

	// TODO
	if !c.outboundBuffer.IsEmpty() {
		_ = el.poller.AddWrite(c.fd)
	}

	return el.handleAction(c, action)
}

func (el *eventloop) loopRead(c *conn) error {
	n, err := unix.Read(c.fd, el.packet)
	if n == 0 || err != nil {
		if err == unix.EAGAIN {
			return nil
		}
		// n = 0 表示连接已关闭
		return el.loopCloseConn(c, err)
	}
	c.buffer = el.packet[:n]

	for inFrame, _ := c.read(); inFrame != nil; inFrame, _ = c.read() {
		out, action := el.eventHandler.React(inFrame, c)
		if out != nil {
			outFrame, _ := el.codec.Encode(c, out)
			el.eventHandler.PreWrite()
			c.write(outFrame)
		}
		switch action {
		case None:
		case Close:
			return el.loopCloseConn(c, nil)
		case Shutdown:
			return errServerShutdown
		}
		if !c.opened {
			return nil
		}
	}
	_, _ = c.inboundBuffer.Write(c.buffer)
	return nil
}

func (el *eventloop) loopReadUDP(fd int) error {
	n, sa , err := unix.Recvfrom(fd, el.packet, 0)
	if err != nil || n == 0 {
		if err != nil && err != unix.EAGAIN {
			el.svr.logger.Printf("failed to read UDP packet from fd:%d, error:%v\n", fd, err)
		}
		return nil
	}
	c := newUDPConn(fd, el, sa)
	out, action := el.eventHandler.React(el.packet[:n], c)
	if out != nil {
		el.eventHandler.PreWrite()
		_ = c.sendTo(out)
	}
	switch action {
	case Shutdown:
		return errServerShutdown
	}
	c.releaseUDP()
	return nil
}

func (el *eventloop) loopAccept(fd int) error {
	if fd == el.svr.ln.fd {
		if el.svr.ln.pconn != nil {
			return el.loopReadUDP(fd)
		}

		nfd, sa, err := unix.Accept(fd)
		if err != nil {
			if err == unix.EAGAIN {
				return nil
			}
			return err
		}
		if err = unix.SetNonblock(nfd, true); err != nil {
			return err
		}
		c := newTCPConn(nfd, el, sa)
		if err = el.poller.AddRead(fd); err == nil {
			el.connections[c.fd] = c
			el.calibrateCallback(el, 1)
			return el.loopOpen(c)
		}
		return err
	}
	return nil
}

package gnet

import (
	"net"

	"golang.org/x/sys/unix"
	"golang_project_note/gnet/internal/netpoll"
	"golang_project_note/gnet/pool/bytebuffer"
	prb "golang_project_note/gnet/pool/ringbuffer"
	"golang_project_note/gnet/ringbuffer"
)

type conn struct {
	fd int
	// remote socket address, accept()返回的
	sa         unix.Sockaddr
	localAddr  net.Addr
	remoteAddr net.Addr
	ctx        interface{}
	// reuse memory of inbound data as a temporary buffer
	// TODO el.loopRead() 首先会将unix.Read()的数据存入buffer中
	buffer []byte
	loop   *eventloop
	opened bool
	codec  ICodec
	// TODO bytes buffer for buffering current packet and data in ring-buffer
	byteBuffer *bytebuffer.ByteBuffer
	// TODO 从客户端接收到的数据
	inboundBuffer *ringbuffer.RingBuffer
	// 发送给客户端的缓冲区，write不完会放到缓冲里
	outboundBuffer *ringbuffer.RingBuffer
}

func newTCPConn(fd int, el *eventloop, sa unix.Sockaddr) *conn {
	return &conn{
		fd:             fd,
		sa:             sa,
		loop:           el,
		codec:          el.codec,
		inboundBuffer:  prb.Get(),
		outboundBuffer: prb.Get(),
	}
}

func (c *conn) releaseTCP() {
	c.opened = false
	c.sa = nil
	c.ctx = nil
	c.buffer = nil
	c.localAddr = nil
	c.remoteAddr = nil
	prb.Put(c.inboundBuffer)
	prb.Put(c.outboundBuffer)
	c.inboundBuffer = nil
	c.outboundBuffer = nil
	bytebuffer.Put(c.byteBuffer)
	c.byteBuffer = nil
}

func newUDPConn(fd int, el *eventloop, sa unix.Sockaddr) *conn {
	return &conn{
		fd:         fd,
		sa:         sa,
		localAddr:  el.svr.ln.lnaddr,
		remoteAddr: netpoll.SockaddrToUDPAddr(sa),
	}
}

func (c *conn) releaseUDP() {
	c.ctx = nil
	c.localAddr = nil
	c.remoteAddr = nil
}

// 如果 el.eventHandler.OnOpened() 有需要返回给client的，会调用open来处理
func (c *conn) open(buf []byte) {
	n, err := unix.Write(c.fd, buf)
	if err != nil {
		_, _ = c.outboundBuffer.Write(buf)
		return
	}

	if n < len(buf) {
		_, _ = c.outboundBuffer.Write(buf[n:])
	}
}

func (c *conn) read() ([]byte, error) {
	return c.codec.Decode(c)
}

func (c *conn) write(buf []byte) {
	if !c.outboundBuffer.IsEmpty() {
		_, _ = c.outboundBuffer.Write(buf)
		return
	}
	n, err := unix.Write(c.fd, buf)
	if err != nil {
		if err == unix.EAGAIN {
			_, _ = c.outboundBuffer.Write(buf)
			_ = c.loop.poller.ModReadWrite(c.fd)
			return
		}
		_ = c.loop.loopCloseConn(c, err)
		return
	}
	if n < len(buf) {
		_, _ = c.outboundBuffer.Write(buf[n:])
		_ = c.loop.poller.ModReadWrite(c.fd)
	}
}

// unix.Sendto: 当flags为0时，Sendto和Write一样
func (c *conn) sendTo(buf []byte) error {
	return unix.Sendto(c.fd, buf, 0, c.sa)
}

func (c *conn) Read() []byte {
	if c.inboundBuffer.IsEmpty() {
		return c.buffer
	}
	// 读取 c.inboundBuffer + c.buffer
	c.byteBuffer = c.inboundBuffer.WithByteBuffer(c.buffer)
	return c.byteBuffer.Bytes()
}

// 清空接收缓冲区
func (c *conn) ResetBuffer() {
	c.buffer = c.buffer[:0]
	c.inboundBuffer.Reset()
	bytebuffer.Put(c.byteBuffer)
	c.byteBuffer = nil
}

func (c *conn) ReadN(n int) (size int, buf []byte) {
	inBufferLen := c.inboundBuffer.Length()
	tempBufferLen := len(c.buffer)
	// n 不会超过已有buffer的大小
	// 此时n是可以读取的大小
	if totalLen := inBufferLen + tempBufferLen; totalLen < n || n <= 0 {
		n = totalLen
	}
	size = n
	if c.inboundBuffer.IsEmpty() {
		buf = c.buffer[:n]
		return
	}
	// c.byteBuffer装的是n个字节
	head, tail := c.inboundBuffer.LazyRead(n)
	c.byteBuffer = bytebuffer.Get()
	_, _ = c.byteBuffer.Write(head)
	_, _ = c.byteBuffer.Write(tail)
	// 可以从byteBuffer中一次性取完
	if inBufferLen >= n {
		buf = c.byteBuffer.Bytes()
		return
	}

	// 剩余的从c.buffer中取
	restSize := n - inBufferLen
	_, _ = c.byteBuffer.Write(c.buffer[:restSize])
	buf = c.byteBuffer.Bytes()
	return
}

func (c *conn) ShiftN(n int) (size int) {
	inBufferLen := c.inboundBuffer.Length()
	tempBufferLen := len(c.buffer)
	if inBufferLen+tempBufferLen < n || n <= 0 {
		c.ResetBuffer()
		size = inBufferLen + tempBufferLen
		return
	}
	size = n
	if c.inboundBuffer.IsEmpty() {
		c.buffer = c.buffer[n:]
		return
	}

	bytebuffer.Put(c.byteBuffer)
	c.byteBuffer = nil
	if inBufferLen > n {
		c.inboundBuffer.Shift(n)
		return
	}

	restSize := n - inBufferLen
	c.buffer = c.buffer[restSize:]
	return
}

func (c *conn) BufferLength() int {
	return c.inboundBuffer.Len() + len(c.buffer)
}

func (c *conn) AsyncWrite(buf []byte) (err error) {
	var encodeBuf []byte
	if encodeBuf, err = c.codec.Encode(c, buf); err == nil {
		return c.loop.poller.Trigger(func() error {
			if c.opened {
				c.write(encodeBuf)
			}
			return nil
		})
	}
	return
}

func (c *conn) SendTo(buf []byte) error {
	return c.sendTo(buf)
}

func (c *conn) Wake() error {
	return c.loop.poller.Trigger(func() error {
		return c.loop.loopWake(c)
	})
}

func (c *conn) Close() error {
	return c.loop.poller.Trigger(func() error {
		return c.loop.loopCloseConn(c, nil)
	})
}

func (c *conn) Context() interface{}       { return c.ctx }
func (c *conn) SetContext(ctx interface{}) { c.ctx = ctx }
func (c *conn) LocalAddr() net.Addr        { return c.localAddr }
func (c *conn) RemoteAddr() net.Addr       { return c.remoteAddr }

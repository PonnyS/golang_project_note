package gnet

import (
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"golang_project_note/gnet/internal/netpoll"
)

// 一个event完成之后需要做的操作
type Action int

const (
	// 啥都不做
	None Action = iota
	// 关闭连接
	Close
	// 关闭server
	Shutdown
)

var defaultLogger = Logger(log.New(os.Stderr, "", log.LstdFlags))

type Logger interface {
	Printf(format string, args ...interface{})
}

type Server struct {
	svr *server
	// 是否启用多核，如果启用则需要注意事件回调之间共享的数据同步
	Multicore bool
	// 服务监听地址
	Addr net.Addr
	// Reactor数
	NumEventLoop int
	// SO_REUSEPORT：支持多个进程/线程绑定到统一端口，这样就不用listen同一个socket
	ReusePort bool
	// SO_KEEPALIVE
	TCPKeepAlive time.Duration
}

type Conn interface {
}

type (
	EventHandler interface {
		// server准备accept连接的时候调用
		OnInitComplete(server Server) (action Action)
		// 在所有event-loop和连接关闭之后调用
		OnShutdown(server Server)
		// accept一个新连接后调用
		// 可以返回些数据给client
		OnOpened(c Conn) (out []byte, action Action)
		// 连接被关闭时调用
		OnClosed(c Conn, err error) (action Action)
		// 在数据写入socket之前调用
		// 通常用于日志、数据上报
		PreWrite()
		// 当有数据到来时调用
		React(frame []byte, c Conn) (out []byte, action Action)
		// 定时任务
		Tick() (delay time.Duration, action Action)
	}
	EventServer struct {
	}
)

func (es *EventServer) OnInitComplete(server Server) (action Action) {
	return
}

func (es *EventServer) OnShutdown(server Server) {
}

func (es *EventServer) OnOpened(c Conn) (out []byte, action Action) {
	return
}

func (es *EventServer) OnClosed(c Conn, err error) (action Action) {
	return
}

func (es *EventServer) PreWrite() {
}

func (es *EventServer) React(frame []byte, c Conn) (out []byte, action Action) {
	return
}

func (es *EventServer) Tick() (delay time.Duration, action Action) {
	return
}

// Serve开始处理指定地址的事件
func Serve(eventHandler EventHandler, addr string, opts ...Option) (err error) {
	var ln listener
	defer func() {
		ln.close()
		// TODO
	}()

	options := loadOptions(opts...)
	if options.Logger != nil {
		defaultLogger = options.Logger
	}

	ln.network, ln.addr = parseAddr(addr)
	switch ln.network {
	case "udp", "udp4", "udp6":
		if options.ReusePort {
			ln.pconn, err = netpoll.ReusePortListenPacket(ln.network, ln.addr)
		} else {
			ln.pconn, err = net.ListenPacket(ln.network, ln.addr)
		}
	case "unix":
		sniffErrorAndLog(os.RemoveAll(ln.addr))
		if runtime.GOOS == "windows" {
			return ErrUnsupportedPlatform
		}
		fallthrough
	case "tcp", "tcp4", "tcp6":
		if options.ReusePort {
			ln.ln, err = netpoll.ReusePortListen(ln.network, ln.addr)
		} else {
			ln.ln, err = net.Listen(ln.network, ln.addr)
		}
	default:
		err = ErrUnsupportedProtocol
	}
	if err != nil {
		return
	}

	if ln.pconn != nil {
		ln.lnaddr = ln.pconn.LocalAddr()
	} else {
		ln.lnaddr = ln.ln.Addr()
	}

	if err = ln.renormalize(); err != nil {
		return
	}

	return serve(eventHandler, &ln, options)
}

// tcp://192.168.0.1:80
func parseAddr(addr string) (network, address string) {
	network = "tcp"
	address = strings.ToLower(addr)
	if strings.Contains(address, "://") {
		pair := strings.Split(address, "://")
		network = pair[0]
		address = pair[1]
	}
	return
}

func sniffErrorAndLog(err error) {
	if err != nil {
		defaultLogger.Printf(err.Error())
	}
}

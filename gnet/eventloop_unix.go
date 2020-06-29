package gnet

import (
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
	connections       map[int]*conn
	eventHandler      EventHandler
	calibrateCallback func(*eventloop, int32)
}

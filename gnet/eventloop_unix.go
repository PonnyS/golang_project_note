package gnet

import (
	"golang_project_note/gnet/internal/netpoll"
)

type eventloop struct {
	svr               *server
	codec             ICodec
	poller            *netpoll.Poller
	packet            []byte
	connections       map[int]*conn
	eventHandler      EventHandler
	calibrateCallback func(*eventloop, int32)
}

package gnet

import (
	"golang_project_note/gnet/internal/netpoll"
)

func (svr *server) activateMainReactor() {
	defer svr.signalShutdown()

	svr.logger.Printf("main reactor exits with error:%v\n", svr.mainLoop.poller.Polling(func(fd int, filter int16) error {
		return svr.acceptNewConnection(fd)
	}))
}

func (svr *server) activateSubReactor(el *eventloop) {
	defer func() {
		el.closeAllConns()
		if el.idx == 0 && svr.opts.Ticker {
			close(svr.ticktock)
		}
		svr.signalShutdown()
	}()

	if el.idx == 0 && svr.opts.Ticker {
		go el.loopTicker()
	}

	svr.logger.Printf("event-loop:%d exits with error:%v\n", el.idx, el.poller.Polling(func(fd int, filter int16) error {
		if c, ok := el.connections[fd]; ok {
			if filter == netpoll.EVFilterSock {
				return el.loopCloseConn(c, nil)
			}
			switch c.outboundBuffer.IsEmpty() {
			case false:
				// 可写事件发生，又有需要写的数据，则直接write
				if filter == netpoll.EVFilterWrite {
					return el.loopWrite(c)
				}
				return nil
			case true:
				if filter == netpoll.EVFilterRead {
					return el.loopRead(c)
				}
				return nil
			}
		}
		return nil
	}))
}

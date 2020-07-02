package gnet

import (
	"golang_project_note/gnet/internal/netpoll"
)

func (el *eventloop) handleEvent(fd int, filter int16) error {
	if c, ok := el.connections[fd]; ok {
		if filter == netpoll.EVFilterSock {
			return el.loopCloseConn(c, nil)
		}
		switch c.outboundBuffer.IsEmpty() {
		case false:
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
	return el.loopAccept(fd)
}

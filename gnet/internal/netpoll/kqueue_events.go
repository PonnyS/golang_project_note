package netpoll

import (
	"golang.org/x/sys/unix"
)

const (
	InitEvents    = 64
	EVFilterWrite = unix.EVFILT_WRITE
	EVFilterRead  = unix.EVFILT_READ
	// 除了read、write外的事件
	EVFilterSock = -0xd
)

type eventList struct {
	size   int
	events []unix.Kevent_t
}

func newEventList(size int) *eventList {
	return &eventList{
		size:   size,
		events: make([]unix.Kevent_t, size),
	}
}

// why not migrate slice's data?
// It's only called when eventList.events has been all handled.
func (el *eventList) increase() {
	el.size >>= 1
	el.events = make([]unix.Kevent_t, el.size)
}

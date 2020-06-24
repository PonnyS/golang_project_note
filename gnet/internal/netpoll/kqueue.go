package netpoll

import (
	"golang.org/x/sys/unix"
	"golang_project_note/gnet/internal"
)

type Poller struct {
	fd            int
	asyncJobQueue internal.AsyncJobQueue
}

func OpenPoller() (poller *Poller, err error) {
	poller = new(Poller)
	if poller.fd, err = unix.Kqueue(); err != nil {
		poller = nil
		return
	}
	// kevent由(ident、filter)对来唯一标识，
	// unix.Kevent 结合了 epoll_ctl 和 epoll_wait
	// changes：注册的事件
	// events：发生的事件
	if _, err = unix.Kevent(poller.fd, []unix.Kevent_t{{
		Ident:  0,
		Filter: unix.EVFILT_USER,
		Flags:  unix.EV_ADD | unix.EV_CLEAR,
	}}, nil, nil); err != nil {
		_ = poller.Close()
		poller = nil
		return
	}

	poller.asyncJobQueue = internal.NewAsyncJobQueue()
	return
}

func (p *Poller) AddRead(fd int) error {
	if _, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD},
	}, nil, nil); err != nil {
		return err
	}
	return nil
}

func (p *Poller) AddWrite(fd int) error {
	if _, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_WRITE, Flags: unix.EV_ADD},
	}, nil, nil); err != nil {
		return err
	}
	return nil
}

func (p *Poller) AddReadWrite(fd int) error {
	if _, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD},
		{Ident: uint64(fd), Filter: unix.EVFILT_WRITE, Flags: unix.EV_ADD},
	}, nil, nil); err != nil {
		return err
	}
	return nil
}

func (p *Poller) Close() error {
	return nil
}

package netpoll

import (
	"log"

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
	// kevent由(ident、filter)对来唯一标识，unix.Kevent 结合了 epoll_ctl 和 epoll_wait
	// changes：注册的事件
	// events：发生的事件
	// 注册了用户事件，将由wakeChanges来触发
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

func (p *Poller) Close() error {
	return unix.Close(p.fd)
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

// 只注册可读事件（可读一直存在），所以需要删除可写事件
func (p *Poller) ModRead(fd int) error {
	if _, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_WRITE, Flags: unix.EV_DELETE},
	}, nil, nil); err != nil {
		return err
	}
	return nil
}

// 注册可读、可写事件
func (p *Poller) ModReadWrite(fd int) error {
	if _, err := unix.Kevent(p.fd, []unix.Kevent_t{
		{Ident: uint64(fd), Filter: unix.EVFILT_WRITE, Flags: unix.EV_ADD},
	}, nil, nil); err != nil {
		return err
	}
	return nil
}

// kqueue会在fd被关闭后移除该fd上所有注册的事件，所以不需手动删除
// epoll是需要的，因为关闭fd，并不会自动从epoll集合中移除
func (p *Poller) Delete(fd int) error {
	return nil
}

// unix.NOTE_TRIGGER 触发用户自定义事件
var wakeChanges = []unix.Kevent_t{
	{Ident: 0, Filter: unix.EVFILT_USER, Fflags: unix.NOTE_TRIGGER},
}

func (p *Poller) Trigger(job internal.Job) error {
	if p.asyncJobQueue.Push(job) == 1 {
		_, err := unix.Kevent(p.fd, wakeChanges, nil, nil)
		return err
	}
	return nil
}

func (p *Poller) Polling(callback func(fd int, filter int16) error) (err error) {
	el := newEventList(InitEvents)
	var wakenUp bool
	for {
		n, err0 := unix.Kevent(p.fd, nil, el.events, nil)
		// EINTR发生在系统调用过程中有信号发生的情况，并没有实际错误发生
		if err0 != nil && err0 != unix.EINTR {
			log.Println(err0)
			continue
		}
		var evFilter int16
		for i := 0; i < n; i++ {
			if fd := int(el.events[i].Ident); fd != 0 {
				evFilter = el.events[i].Filter
				if (el.events[i].Flags&unix.EV_EOF != 0) || (el.events[i].Flags&unix.EV_ERROR != 0) {
					evFilter = EvFilterSock
				}

				if err = callback(fd, evFilter); err != nil {
					return
				}
			} else {
				wakenUp = true
			}
		}

		if wakenUp {
			wakenUp = false
			if err = p.asyncJobQueue.ForEach(); err != nil {
				return
			}
		}
		if n == el.size {
			el.increase()
		}
	}
}

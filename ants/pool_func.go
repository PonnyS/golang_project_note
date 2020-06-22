package ants

import (
	"sync"
	"sync/atomic"
	"time"

	"golang_project_note/ants/internal"
)

type PoolWithFunc struct {
	capacity    int32
	running     int32
	workers     []*goWorkerWithFunc
	state       int32
	lock        sync.Locker
	cond        *sync.Cond
	poolFunc    func(interface{})
	workerCache sync.Pool
	blockingNum int
	options     *Options
}

func NewPoolWithFunc(size int, pf func(interface{}), options ...Option) (*PoolWithFunc, error) {
	if size < 0 {
		return nil, ErrInvalidPoolSize
	}
	if pf == nil {
		return nil, ErrLackPoolFunc
	}

	opts := loadOptions(options...)

	if expiry := opts.ExpiryDuration; expiry < 0 {
		return nil, ErrInvalidPoolExpiry
	} else if expiry == 0 {
		opts.ExpiryDuration = DefaultCleanIntervalTime
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &PoolWithFunc{
		capacity: int32(size),
		poolFunc: pf,
		lock:     internal.NewSpinLock(),
		options:  opts,
	}
	p.workerCache.New = func() interface{} {
		return &goWorkerWithFunc{
			pool: p,
			args: make(chan interface{}, workerChanCap),
		}
	}
	if p.options.PreAlloc {
		p.workers = make([]*goWorkerWithFunc, 0, size)
	}
	p.cond = sync.NewCond(p.lock)

	go p.periodicallyPurge()
	return p, nil
}

func (p *PoolWithFunc) periodicallyPurge() {
	heartbeat := time.NewTicker(p.options.ExpiryDuration)
	defer heartbeat.Stop()

	var expiredWorkers []*goWorkerWithFunc
	for range heartbeat.C {
		if atomic.LoadInt32(&p.state) == CLOSED {
			return
		}
		currentTime := time.Now()
		p.lock.Lock()
		idleWorkers := p.workers
		n := len(idleWorkers)
		var i int
		for i = 0; i < n && currentTime.Sub(idleWorkers[i].recycleTime) > p.options.ExpiryDuration; i++ {
		}
		// 因为for循环，需要[:0]
		expiredWorkers = append(expiredWorkers[:0], idleWorkers[i:]...)
		if i > 0 {
			// 将不过期的worker拷贝到idleWorkers前面
			// m为不过期的worker数
			m := copy(idleWorkers, idleWorkers[i:])
			for i := m; i < n; i++ {
				idleWorkers[i] = nil
			}
			p.workers = idleWorkers[:m]
		}
		p.lock.Unlock()

		for i, w := range expiredWorkers {
			w.args <- nil
			expiredWorkers[i] = nil
		}

		if p.Running() == 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *PoolWithFunc) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *PoolWithFunc) Free() int {
	return p.Cap() - p.Running()
}

func (p *PoolWithFunc) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *PoolWithFunc) Tune(size int) {
	if size < 0 || size == p.Cap() || p.options.PreAlloc {
		return
	}
	atomic.StoreInt32(&p.capacity, int32(size))
}

func (p *PoolWithFunc) Release() {
	atomic.StoreInt32(&p.state, CLOSED)
	p.lock.Lock()
	idleWorkers := p.workers
	for _, w := range idleWorkers {
		w.args <- nil
	}
	p.workers = nil
	p.lock.Unlock()
}

func (p *PoolWithFunc) Reboot() {
	if atomic.CompareAndSwapInt32(&p.state, CLOSED, OPENED) {
		go p.periodicallyPurge()
	}
}

func (p *PoolWithFunc) incRunning() {
	atomic.AddInt32(&p.running, 1)
}

func (p *PoolWithFunc) decRunning() {
	atomic.AddInt32(&p.running, -1)
}

func (p *PoolWithFunc) Invoke(args interface{}) error {
	if atomic.LoadInt32(&p.state) == CLOSED {
		return ErrPoolClosed
	}
	var w *goWorkerWithFunc
	if w = p.retrieveWorker(); w == nil {
		return ErrPoolOverload
	}
	w.args <- args
	return nil
}

func (p *PoolWithFunc) retrieveWorker() (w *goWorkerWithFunc) {
	spawnWorker := func() {
		w = p.workerCache.Get().(*goWorkerWithFunc)
		w.run()
	}

	p.lock.Lock()
	idleWorkers := p.workers
	n := len(idleWorkers) - 1
	if n >= 0 {
		w = idleWorkers[n]
		idleWorkers[n] = nil
		p.workers = idleWorkers[:n]
		p.lock.Unlock()
	} else if p.Running() < p.Cap() {
		p.lock.Unlock()
		spawnWorker()
	} else {
		if p.options.Nonblocking {
			p.lock.Unlock()
			return
		}
	Reentry:
		if p.options.MaxBlockingTasks != 0 && p.blockingNum >= p.options.MaxBlockingTasks {
			p.lock.Unlock()
			return
		}
		p.blockingNum++
		p.cond.Wait()
		p.blockingNum--
		if p.Running() == 0 {
			p.lock.Unlock()
			spawnWorker()
			return
		}

		l := len(p.workers) - 1
		if l < 0 {
			goto Reentry
		}
		w = p.workers[l]
		p.workers[l] = nil
		p.workers = p.workers[:l]
		p.lock.Unlock()
	}
	return
}

func (p *PoolWithFunc) revertWorker(worker *goWorkerWithFunc) bool {
	if atomic.LoadInt32(&p.state) == CLOSED || p.Running() > p.Cap() {
		return false
	}

	worker.recycleTime = time.Now()
	p.lock.Lock()
	p.workers = append(p.workers, worker)

	p.cond.Signal()
	p.lock.Unlock()
	return true
}

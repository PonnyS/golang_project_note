package ants

import (
	"sync"
	"sync/atomic"
	"time"

	"golang_project_note/ants/internal"
)

type Pool struct {
	// pool的worker（协程）容量
	capacity int32
	// 当前正在运行中的worker（协程）数量
	running int32
	// 并发操作 pool.workers 是需要加锁的
	lock sync.Locker
	// 先尝试从队列中获取worker
	workers workerArray
	// why sync.Pool
	// 1. 减轻GC压力，复用对象内存
	// 2. 来缓存无处安放的worker（队列满了）
	// 3. 承担着生产worker的任务，使用sync.Pool还可以从其他P.shared偷取
	workerCache sync.Pool
	// 无限capacity
	// 用于嵌套任务：submit一个任务，这个任务又会submit一个新任务到pool
	infinite bool
	// option单独一文件
	options *Options
	// 对于达到阻塞数上限的任务设置一个条件变量
	// 因为wait()后再次上锁，所以条件满足后，也只能有一个任务会得到执行
	cond *sync.Cond
	// 使用atomic来进行原子操作
	// atomic的原子性是由底层硬件来保证的，性能更高，更能利用多核优势，而Mutex则是操作系统的调度器来实现
	state int32
	// 阻塞中的任务数
	blockingNum int
}

func NewPool(size int, options ...Option) (*Pool, error) {
	opts := loadOptions(options...)

	if expiry := opts.ExpiryDuration; expiry < 0 {
		return nil, ErrInvalidPoolExpiry
	} else if expiry == 0 {
		opts.ExpiryDuration = DefaultCleanIntervalTime
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &Pool{
		capacity: int32(size),
		lock:     internal.NewSpinLock(),
		options:  opts,
	}
	p.workerCache.New = func() interface{} {
		return &goWorker{
			pool: p,
			task: make(chan func(), workerChanCap),
		}
	}
	if size < 0 {
		p.infinite = true
	}
	if p.options.PreAlloc {
		// make([]*goWorker, size)
		p.workers = newWorkerArray(loopQueueType, size)
	} else {
		// make([]*goWorker, 0, size)
		p.workers = newWorkerArray(stackType, 0)
	}

	p.cond = sync.NewCond(p.lock)

	go p.periodicallyPurge()
	return p, nil
}

// 定时清理过期协程，节省资源
func (p *Pool) periodicallyPurge() {
	// 心跳的实现：time.NewTicker()
	heartbeat := time.NewTicker(p.options.ExpiryDuration)
	defer heartbeat.Stop()

	for range heartbeat.C {
		if atomic.LoadInt32(&p.state) == CLOSED {
			break
		}

		p.lock.Lock()
		expiredWorkers := p.workers.retrieveExpiry(p.options.ExpiryDuration)
		p.lock.Unlock()

		for i := range expiredWorkers {
			expiredWorkers[i].task <- nil
		}

		// 清理完后没有在运行的任务了，直接广播所有的阻塞任务
		// 由于Reboot()操作的可能，workers队列此时可能是空的
		if p.Running() == 0 {
			p.cond.Broadcast()
		}
	}
}

func (p *Pool) Submit(task func()) error {
	if atomic.LoadInt32(&p.state) == CLOSED {
		return ErrPoolClosed
	}
	var w *goWorker
	if w = p.retrieveWorker(); w == nil {
		return ErrPoolOverload
	}
	w.task <- task
	return nil
}

// 清理资源
func (p *Pool) Release() {
	atomic.StoreInt32(&p.state, CLOSED)
	p.lock.Lock()
	// workers的切片置空
	p.workers.reset()
	p.lock.Unlock()
}

func (p *Pool) Reboot() {
	if atomic.CompareAndSwapInt32(&p.state, CLOSED, OPENED) {
		// 在 Release() 的时候 periodicallyPurge() 已经退出了
		go p.periodicallyPurge()
	}
}

func (p *Pool) retrieveWorker() (w *goWorker) {
	spawnWorker := func() {
		w = p.workerCache.Get().(*goWorker)
		w.run()
	}

	p.lock.Lock()
	// w 为nil的两种情况
	// 1. 刚开始，workerArray 是空的
	// 2. workerArray 满了
	w = p.workers.detach()
	if w != nil {
		p.lock.Unlock()
	} else if p.infinite {
		p.lock.Unlock()
		spawnWorker()
	} else if p.Running() < p.Cap() {
		// 出现这种情况的可能：
		// 1. 刚开始，workerArray 是空的
		// TODO：2. workerArray 已经满了，但这时候调整了容量（即 Tune()）
		p.lock.Unlock()
		spawnWorker()
	} else {
		// 无阻塞
		if p.options.NonBlocking {
			p.lock.Unlock()
			return
		}
	Reentry:
		if p.options.MaxBlockingTasks != 0 && p.blockingNum >= p.options.MaxBlockingTasks {
			p.lock.Unlock()
			return
		}
		p.blockingNum++
		// wait()操作会先解锁，再调用 gopark() 把当前协程挂到等待队列中，等待被手动唤醒。
		// 唤醒后会重新上锁，此时可能锁被抢占导致再次阻塞
		p.cond.Wait()
		p.blockingNum--
		// 结合 *goWorker.run() 来看，decRunning()后会将 *goWorker 放入 workerCache 中，所以直接从 workerCache 中来拿即可
		// 结合 periodicallyPurge() 来看，是因为把所有 worker 都清理了
		if p.Running() == 0 {
			p.lock.Unlock()
			spawnWorker()
		}

		w = p.workers.detach()
		if w == nil {
			goto Reentry
		}
		p.lock.Unlock()
	}

	return
}

func (p *Pool) revertWorker(w *goWorker) bool {
	if atomic.LoadInt32(&p.state) == CLOSED || (!p.infinite && p.Running() > p.Cap()) {
		return false
	}
	p.lock.Lock()
	err := p.workers.insert(w)
	if err != nil {
		p.lock.Unlock()
		return false
	}

	// 只通知一个
	p.cond.Signal()
	p.lock.Unlock()
	return true
}

func (p *Pool) incRunning() {
	atomic.AddInt32(&p.running, 1)
}

func (p *Pool) decRunning() {
	atomic.AddInt32(&p.running, -1)
}

func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

func (p *Pool) Free() int {
	return p.Cap() - p.Running()
}

func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

func (p *Pool) Tune(size int) {
	if size < 0 || p.Cap() == size || p.options.PreAlloc {
		return
	}
	atomic.StoreInt32(&p.capacity, int32(size))
}

package ants

import (
	"errors"
	"log"
	"math"
	"os"
	"runtime"
	"time"
)

const (
	DefaultCleanIntervalTime = time.Second

	DefaultAntsPoolSize = math.MaxInt32
)

const (
	OPENED = iota
	CLOSED
)

var (
	// ErrInvalidPoolSize will be returned when setting a negative number as pool capacity.
	ErrInvalidPoolSize = errors.New("invalid size for pool")

	// ErrLackPoolFunc will be returned when invokers don't provide function for pool.
	ErrLackPoolFunc = errors.New("must provide function for pool")

	// ErrInvalidPoolExpiry will be returned when setting a negative number as the periodic duration to purge goroutines.
	ErrInvalidPoolExpiry = errors.New("invalid expiry for pool")

	// ErrPoolClosed will be returned when submitting task to a closed pool.
	ErrPoolClosed = errors.New("this pool has been closed")

	// ErrPoolOverload will be returned when the pool is full and no workers available.
	ErrPoolOverload = errors.New("too many goroutines blocked on submit or Nonblocking is set")

	ErrInvalidPreAllocSize = errors.New("can not set up a negative capacity under PreAlloc mode")

	defaultLogger = Logger(log.New(os.Stderr, "", log.LstdFlags))

	defaultAntsPool, _ = NewPool(DefaultAntsPoolSize)

	//你对 ants 的调度模式的理解的就是错的，调用 Submit 方法提交任务的时候，同一个 worker 只能被一个调用方持有，
	//不存在多个调用方都可以拿到同一个 worker 并往它的 chan 里塞任务。
	//一个 worker 如果被一个调用方取得，则拿到该 worker 的下一个调用方肯定是在上一个调用方的任务已经被执行完之后，
	//所以对于多核机器来说，worker 的 chan buffer 大小为 1 已经是最佳长度了，大于 1 是完全没必要的。
	//如果要达到你期望的效果，那不是仅仅修改 workerChanCap 就可以的，整个调度模式都要改变。
	//最后，并发量上不去和 workerChanCap 的长度没有直接关系吧，就算是按照你的想法一个 worker 可以缓存多个任务，整体的处理速度也不会变快，
	//如果你仅仅是想要提升吞吐量，可以把 worker 的数量设置多一点，至于缓存任务可以放在业务层去做。
	// Submit 提交任务的时候获取 worker，run()结束后 revertWorker() 放回 worker，所以 chan 缓存大于 1 是没必要的
	workerChanCap = func() int {
		if runtime.GOMAXPROCS(0) == 1 {
			return 0
		}
		return 1
	}()
)

type Logger interface {
	Printf(format string, args ...interface{})
}

func Submit(task func()) error {
	return defaultAntsPool.Submit(task)
}

func Running() int {
	return defaultAntsPool.Running()
}

func Cap() int {
	return defaultAntsPool.Cap()
}

func Free() int {
	return defaultAntsPool.Free()
}

func Release() {
	defaultAntsPool.Release()
}

func Reboot() {
	defaultAntsPool.Reboot()
}

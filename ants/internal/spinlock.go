// 为什么不用sync.Mutex，而是自己实现自旋锁
//
// sync.Mutex: 调用gopark()将当前 G 以 _Gwaiting 状态放入等待队列中，必须调用 goready() 来唤醒
// Gosched()：将当前 G 以 _Grunnable 状态放入全局队列中，可以直接被 M 来调度
//
// 结合spinlock的使用场景，锁会迅速被释放，所以无需用sync.Mutex徒增调度压力

package internal

import (
	"runtime"
	"sync"
	"sync/atomic"
)

type spinlock uint32

func (sl *spinlock) Lock() {
	for !atomic.CompareAndSwapUint32((*uint32)(sl), 0, 1) {
		runtime.Gosched()
	}
}

func (sl *spinlock) Unlock() {
	atomic.StoreUint32((*uint32)(sl), 0)
}

func NewSpinLock() sync.Locker {
	return new(spinlock)
}

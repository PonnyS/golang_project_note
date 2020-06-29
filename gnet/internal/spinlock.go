package internal

import (
	"runtime"
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

func Spinlock() *spinlock {
	return new(spinlock)
}

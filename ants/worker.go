package ants

import (
	"runtime"
	"time"
)

type goWorker struct {
	pool        *Pool
	task        chan func()
	recycleTime time.Time
}

func (w *goWorker) run() {
	w.pool.incRunning()
	go func() {
		defer func() {
			w.pool.decRunning()
			// 放入sync.Pool中，对象复用，有利于缓解GC压力，其他P也可以来偷取worker来使用
			w.pool.workerCache.Put(w)
			if p := recover(); p != nil {
				if ph := w.pool.options.PanicHandler; ph != nil {
					ph(p)
				} else {
					w.pool.options.Logger.Printf("worker exits from a panic: %v\n", p)
					var buf [4096]byte
					n := runtime.Stack(buf[:], false)
					w.pool.options.Logger.Printf("worker exits from panic: %s\n", string(buf[:n]))
				}
			}
		}()

		for f := range w.task {
			// 为什么worker退出不放回队列？
			// 因为退出有两种情况：1. 清理过期协程 2. 队列重置。
			if f == nil {
				return
			}
			f()
			// 每个任务独占一个worker，用完即放回队列中，相比放回workerCache提高效率
			if ok := w.pool.revertWorker(w); !ok {
				return
			}
		}
	}()
}

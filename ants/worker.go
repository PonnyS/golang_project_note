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
			// 两种情况
			// 1. 正常退出：worker是没有放回队列的
			// 2. 队列满了：放入workerCache（sync.Pool）中，有shared区和private区
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
			// 退出是不需要把worker放回队列的
			if f == nil {
				return
			}
			f()
			// 每个任务独占一个worker，用完即放回队列中重复使用
			if ok := w.pool.revertWorker(w); !ok {
				return
			}
		}
	}()
}

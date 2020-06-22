package ants

import "time"

// 循环队列，事先分配好数组的内存
type loopQueue struct {
	items  []*goWorker
	expiry []*goWorker
	size   int
	// 指向可以detach的元素
	head int
	// 指向可以insert的元素
	tail   int
	isFull bool
}

func newWorkerLoopQueue(size int) *loopQueue {
	return &loopQueue{
		items: make([]*goWorker, size),
		size:  size,
	}
}

func (wq *loopQueue) retrieveExpiry(duration time.Duration) []*goWorker {
	if wq.isEmpty() {
		return nil
	}

	wq.expiry = wq.expiry[:0]
	expiryTime := time.Now().Add(-duration)

	for !wq.isEmpty() {
		if expiryTime.Before(wq.items[wq.head].recycleTime) {
			break
		}
		wq.expiry = append(wq.expiry, wq.items[wq.head])
		// 相当于该元素已被获取，可以覆盖
		wq.head++
		if wq.head == wq.size {
			wq.head = 0
		}
		wq.isFull = false
	}

	return wq.expiry
}

func (wq *loopQueue) reset() {
	if wq.isEmpty() {
		return
	}
Releasing:
	if w := wq.detach(); w != nil {
		w.task <- nil
		goto Releasing
	}
	wq.items = wq.items[:0]
	wq.size = 0
	wq.head = 0
	wq.tail = 0
}

func (wq *loopQueue) insert(worker *goWorker) error {
	if wq.size == 0 {
		return errQueueIsReleased
	}
	if wq.isFull {
		return errQueueIsFull
	}

	wq.items[wq.tail] = worker
	wq.tail++

	if wq.tail == wq.size {
		wq.tail = 0
	}
	if wq.tail == wq.head {
		wq.isFull = true
	}
	return nil
}

func (wq *loopQueue) isEmpty() bool {
	return wq.head == wq.tail && !wq.isFull
}

func (wq *loopQueue) len() int {
	if wq.size == 0 {
		return 0
	}

	if wq.head == wq.tail {
		if wq.isFull {
			return wq.size
		}
		return 0
	}

	if wq.tail > wq.head {
		return wq.tail - wq.head
	}
	return wq.size - wq.head + wq.tail
}

func (wq *loopQueue) detach() *goWorker {
	if wq.isEmpty() {
		return nil
	}

	w := wq.items[wq.head]
	wq.head++
	if wq.head == wq.size {
		wq.head = 0
	}
	wq.isFull = false

	return w
}

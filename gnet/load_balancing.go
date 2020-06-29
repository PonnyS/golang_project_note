package gnet

import (
	"container/heap"
	"sync"
	"sync/atomic"
)

type LoadBalancing int

const (
	RoundRobin LoadBalancing = iota
	LeastConnections
	SourceAddrHash
)

type (
	loadBalancer interface {
		register(*eventloop)
		next(int) *eventloop
		iterate(func(int, *eventloop) bool)
		len() int
		calibrate(*eventloop, int32)
	}

	roundRobinEventLoopSet struct {
		nextLoopIndex int
		size          int
		eventLoops    []*eventloop
	}

	leastConnectionsEventLoopSet struct {
		// 涉及到了slice的操作
		sync.RWMutex
		// 最小堆数据结构，实现了container/heap
		minHeap minEventLoopHeap
		// cachedRoot为每次next()获取到的对象
		cachedRoot *eventloop
		// 可以理解为connection数，在重建最小堆后会重新置零
		threshold int32
		// 最小堆的元素数，即eventloop数
		calibrateConnsThreshold int32
	}

	sourceAddrHashEventLoopSet struct {
		eventLoops []*eventloop
		size       int
	}
)

// ==================================== Implementation of Round-Robin load-balancer ====================================
func (set *roundRobinEventLoopSet) register(el *eventloop) {
	el.idx = set.size
	set.eventLoops = append(set.eventLoops, el)
	set.size++
}

func (set *roundRobinEventLoopSet) next(_ int) (el *eventloop) {
	el = set.eventLoops[set.nextLoopIndex]
	if set.nextLoopIndex++; set.nextLoopIndex >= set.size {
		set.nextLoopIndex = 0
	}
	return
}

func (set *roundRobinEventLoopSet) iterate(f func(int, *eventloop) bool) {
	for i, el := range set.eventLoops {
		if !f(i, el) {
			break
		}
	}
}

func (set *roundRobinEventLoopSet) len() int {
	return set.size
}

func (set *roundRobinEventLoopSet) calibrate(el *eventloop, delta int32) {
	atomic.AddInt32(&el.connCount, delta)
}

// ================================= Implementation of Least-Connections load-balancer =================================
type minEventLoopHeap []*eventloop

func (h minEventLoopHeap) Len() int {
	return len(h)
}

func (h minEventLoopHeap) Less(i, j int) bool {
	return h[i].connCount < h[j].connCount
}

func (h minEventLoopHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].idx, h[j].idx = i, j
}

func (h *minEventLoopHeap) Push(x interface{}) {
	el := x.(*eventloop)
	el.idx = len(*h)
	*h = append(*h, el)
}

func (h *minEventLoopHeap) Pop() interface{} {
	old := *h
	i := len(old) - 1
	x := old[i]
	// 防止内存泄露
	old[i] = nil
	// 安全起见
	x.idx = -1
	*h = old[:i]
	return x
}

func (set *leastConnectionsEventLoopSet) register(el *eventloop) {
	set.Lock()
	heap.Push(&set.minHeap, el)
	if el.idx == 0 {
		set.cachedRoot = el
	}
	set.calibrateConnsThreshold = int32(set.minHeap.Len())
	set.Unlock()
}

func (set *leastConnectionsEventLoopSet) next(i int) *eventloop {
	// 每 calibrateConnsThreshold 次会重建最小堆，这样能减少锁的使用
	if atomic.LoadInt32(&set.threshold) >= set.calibrateConnsThreshold {
		set.Lock()
		heap.Init(&set.minHeap)
		set.cachedRoot = set.minHeap[0]
		atomic.StoreInt32(&set.threshold, 0)
		set.Unlock()
	}
	return set.cachedRoot
}

func (set *leastConnectionsEventLoopSet) iterate(f func(int, *eventloop) bool) {
	set.RLock()
	for i, el := range set.minHeap {
		if !f(i, el) {
			break
		}
	}
	set.RUnlock()
}

func (set *leastConnectionsEventLoopSet) len() (size int) {
	set.Lock()
	size = set.minHeap.Len()
	set.Unlock()
	return
}

func (set *leastConnectionsEventLoopSet) calibrate(el *eventloop, delta int32) {
	set.Lock()
	atomic.AddInt32(&el.connCount, delta)
	atomic.AddInt32(&set.threshold, 1)
	set.Unlock()
}

// ======================================= Implementation of Hash load-balancer ========================================
func (set *sourceAddrHashEventLoopSet) register(el *eventloop) {
	el.idx = set.size
	set.eventLoops = append(set.eventLoops, el)
	set.size++
}

func (set *sourceAddrHashEventLoopSet) next(hashcode int) *eventloop {
	return set.eventLoops[hashcode%set.size]
}

func (set *sourceAddrHashEventLoopSet) iterate(f func(int, *eventloop) bool) {
	for i, el := range set.eventLoops {
		if !f(i, el) {
			break
		}
	}
}

func (set *sourceAddrHashEventLoopSet) len() int {
	return set.size
}

func (set *sourceAddrHashEventLoopSet) calibrate(el *eventloop, delta int32) {
	atomic.AddInt32(&el.connCount, delta)
}

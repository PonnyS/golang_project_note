package gnet

type LoadBalancing int

const (
	RoundRobin LoadBalancing = iota
	LeastConnections
	SourceAddrHash
)

type (
	loadBalancer interface {
		register(*eventloop)
		calibrate(*eventloop, int32)
	}

	roundRobinEventLoopSet struct {
	}

	leastConnectionsEventLoopSet struct {
	}

	sourceAddrHashEventLoopSet struct {
	}
)

// ==================================== Implementation of Round-Robin load-balancer ====================================
func (set *roundRobinEventLoopSet) register(el *eventloop) {
	panic("implement me")
}

func (set *roundRobinEventLoopSet) calibrate(el *eventloop, delta int32) {
	panic("implement me")
}

// ================================= Implementation of Least-Connections load-balancer =================================
func (set *leastConnectionsEventLoopSet) register(el *eventloop) {
	panic("implement me")
}

func (set *leastConnectionsEventLoopSet) calibrate(el *eventloop, delta int32) {
	panic("implement me")
}

// ======================================= Implementation of Hash load-balancer ========================================
func (set *sourceAddrHashEventLoopSet) register(el *eventloop) {
	panic("implement me")
}

func (set *sourceAddrHashEventLoopSet) calibrate(el *eventloop, delta int32) {
	panic("implement me")
}

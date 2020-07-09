package gnet

import (
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"golang_project_note/gnet/internal/netpoll"
)

type server struct {
	ln              *listener
	opts            *Options
	once            sync.Once
	cond            *sync.Cond
	wg              sync.WaitGroup
	mainLoop        *eventloop
	logger          Logger
	eventHandler    EventHandler
	subEventLoopSet loadBalancer
	ticktock        chan time.Duration
	codec           ICodec
}

func (svr *server) start(numEventLoop int) error {
	if svr.opts.ReusePort || svr.ln.pconn != nil {
		return svr.activateLoops(numEventLoop)
	}
	return svr.activateReactors(numEventLoop)
}

func (svr *server) activateReactors(numEventLoop int) error {
	for i := 0; i < numEventLoop; i++ {
		if p, err := netpoll.OpenPoller(); err == nil {
			el := &eventloop{
				svr:               svr,
				codec:             svr.codec,
				poller:            p,
				packet:            make([]byte, 0x10000),
				connections:       make(map[int]*conn),
				eventHandler:      svr.eventHandler,
				calibrateCallback: svr.subEventLoopSet.calibrate,
			}
			svr.subEventLoopSet.register(el)
		} else {
			return err
		}
	}

	// Why startReactors before main reactor begin?
	svr.startReactors()

	if p, err := netpoll.OpenPoller(); err == nil {
		el := &eventloop{
			idx:    -1,
			poller: p,
			svr:    svr,
		}
		_ = el.poller.AddRead(svr.ln.fd)
		svr.mainLoop = el
		svr.wg.Add(1)
		go func() {
			svr.activateMainReactor()
			svr.wg.Done()
		}()
	} else {
		return err
	}
	return nil
}

func (svr *server) activateLoops(numEventLoop int) error {
	for i := 0; i < numEventLoop; i++ {
		if p, err := netpoll.OpenPoller(); err == nil {
			el := &eventloop{
				svr:               svr,
				codec:             svr.codec,
				poller:            p,
				packet:            make([]byte, 0x10000),
				connections:       make(map[int]*conn),
				eventHandler:      svr.eventHandler,
				calibrateCallback: svr.subEventLoopSet.calibrate,
			}
			_ = el.poller.AddRead(svr.ln.fd)
			svr.subEventLoopSet.register(el)
		} else {
			return err
		}
	}
	svr.startLoops()
	return nil
}

func (svr *server) startReactors() {
	svr.subEventLoopSet.iterate(func(i int, e *eventloop) bool {
		svr.wg.Add(1)
		go func() {
			svr.activateSubReactor(e)
			svr.wg.Done()
		}()
		return true
	})
}

func (svr *server) startLoops() {
	svr.subEventLoopSet.iterate(func(i int, e *eventloop) bool {
		svr.wg.Add(1)
		go func() {
			e.loopRun()
			svr.wg.Done()
		}()
		return true
	})
}

func (svr *server) waitForShutdown() {
	// unlock未lock的锁会fatal error
	svr.cond.L.Lock()
	// TODO 会产生死锁
	svr.cond.Wait()
	svr.cond.L.Unlock()
}

func (svr *server) signalShutdown() {
	svr.once.Do(func() {
		svr.cond.L.Lock()
		svr.cond.Signal()
		svr.cond.L.Unlock()
	})
}

func (svr *server) stop() {
	svr.waitForShutdown()

	svr.subEventLoopSet.iterate(func(i int, e *eventloop) bool {
		sniffErrorAndLog(e.poller.Trigger(func() error {
			return errServerShutdown
		}))
		return true
	})

	if svr.mainLoop != nil {
		svr.ln.close()
		sniffErrorAndLog(svr.mainLoop.poller.Trigger(func() error {
			return errServerShutdown
		}))
	}

	svr.wg.Wait()

	svr.closeLoops()

	if svr.mainLoop != nil {
		sniffErrorAndLog(svr.mainLoop.poller.Close())
	}
}

func (svr *server) closeLoops() {
	svr.subEventLoopSet.iterate(func(i int, e *eventloop) bool {
		_ = e.poller.Close()
		return true
	})
}

func serve(eventHandler EventHandler, listener *listener, options *Options) error {
	numEventLoop := 1
	if options.Multicore {
		numEventLoop = runtime.NumCPU()
	}
	if options.NumEventLoop > 0 {
		numEventLoop = options.NumEventLoop
	}

	svr := new(server)
	svr.eventHandler = eventHandler
	svr.opts = options
	svr.ln = listener

	switch options.LB {
	case RoundRobin:
		svr.subEventLoopSet = new(roundRobinEventLoopSet)
	case LeastConnections:
		svr.subEventLoopSet = new(leastConnectionsEventLoopSet)
	case SourceAddrHash:
		svr.subEventLoopSet = new(sourceAddrHashEventLoopSet)
	}

	svr.cond = sync.NewCond(&sync.Mutex{})
	svr.ticktock = make(chan time.Duration, 1)
	svr.logger = func() Logger {
		if options.Logger == nil {
			return defaultLogger
		}
		return options.Logger
	}()

	svr.codec = func() ICodec {
		if options.Codec == nil {
			return new(BuiltInFrameCodec)
		}
		return options.Codec
	}()

	server := Server{
		svr:          svr,
		Multicore:    options.Multicore,
		Addr:         listener.lnaddr,
		NumEventLoop: numEventLoop,
		ReusePort:    options.ReusePort,
		TCPKeepAlive: options.TCPKeepAlive,
	}
	switch svr.eventHandler.OnInitComplete(server) {
	case None:
	case Shutdown:
		return nil
	}
	defer svr.eventHandler.OnShutdown(server)

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer close(shutdown)

	go func() {
		if <-shutdown == nil {
			return
		}
		svr.signalShutdown()
	}()

	if err := svr.start(numEventLoop); err != nil {
		svr.closeLoops()
		svr.logger.Printf("gnet server is stoping with error: %v\n", err)
		return err
	}
	defer svr.stop()

	return nil
}

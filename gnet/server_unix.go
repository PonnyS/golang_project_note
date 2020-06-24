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
				poller: p,
				// 	TODO
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
			poller: p,
			// TODO
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
	return nil
}

func (svr *server) startReactors() {

}

func (svr *server) stop() {

}

func (svr *server) closeLoops() {

}

func (svr *server) signalShutdown() {

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
	// TODO
	// svr.codec = func() ICodec {
	// 	if options.Codec == nil {
	// 		return new(BuiltInFrameCodec)
	// 	}
	// 	return options.Codec
	// }()

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

package main

import (
	"sync"
	"time"
)

var m = &sync.Mutex{}
var cond = sync.NewCond(m)

func waitForShutdown() {
	cond.L.Lock()
	cond.Wait()
	cond.L.Unlock()
}

func signalShutdown() {
	cond.L.Lock()
	cond.Signal()
	cond.L.Unlock()
}

func f() {
	defer waitForShutdown()
	time.Sleep(100*time.Millisecond)
}

func main() {
	f()

	time.Sleep(10*time.Millisecond)
	signalShutdown()
	return
}

package goroutine

import (
	"time"

	"golang_project_note/ants"
)

const (
	DefaultAntsPoolSize = 1 << 18

	ExpiryDuration = 10 * time.Second

	Nonblocking = true
)

func init() {
	ants.Release()
}

type Pool = ants.Pool

func Default() *Pool {
	options := ants.Options{
		Nonblocking: Nonblocking,
		ExpiryDuration: ExpiryDuration,
	}
	defaultAntsPool, _ := ants.NewPool(DefaultAntsPoolSize, ants.WithOptions(options))
	return defaultAntsPool
}

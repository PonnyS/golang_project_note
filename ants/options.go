package ants

import "time"

type Options struct {
	ExpiryDuration time.Duration
	Logger         Logger
	// 是否在初始化 pool 的时候就给 worker数组分配好 `items []*goWorker` 内存
	PreAlloc         bool
	PanicHandler     func(interface{})
	Nonblocking      bool
	MaxBlockingTasks int
}

type Option func(opts *Options)

func loadOptions(options ...Option) *Options {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	return opts
}

func WithOptions(options Options) Option {
	return func(opts *Options) {
		*opts = options
	}
}

func WithExpiryDuration(expiryDuration time.Duration) Option {
	return func(opts *Options) {
		opts.ExpiryDuration = expiryDuration
	}
}

func WithLogger(logger Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}

func WithPreAlloc(preAlloc bool) Option {
	return func(opts *Options) {
		opts.PreAlloc = preAlloc
	}
}

func WithPanicHandler(panicHandler func(interface{})) Option {
	return func(opts *Options) {
		opts.PanicHandler = panicHandler
	}
}

func WithNonblocking(nonBlocking bool) Option {
	return func(opts *Options) {
		opts.Nonblocking = nonBlocking
	}
}

func WithMaxBlockingTasks(maxBlockingTasks int) Option {
	return func(opts *Options) {
		opts.MaxBlockingTasks = maxBlockingTasks
	}
}

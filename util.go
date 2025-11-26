package main

import (
	"net"
	"sync"
	"sync/atomic"

	"golang.org/x/time/rate"
)

const maxEntries = 10000 // Rotate when current map reaches this size (~2.5MB)

var (
	current      = &sync.Map{}
	previous     = &sync.Map{}
	currentCount int64
)

func rateLimitAllow(addr string) bool {
	ip := addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		ip = host
	}

	if atomic.LoadInt64(&currentCount) >= maxEntries {
		rotate()
	}

	if val, ok := current.Load(ip); ok {
		return val.(*rate.Limiter).Allow()
	}

	if val, ok := previous.Load(ip); ok {
		current.Store(ip, val)
		atomic.AddInt64(&currentCount, 1)
		return val.(*rate.Limiter).Allow()
	}

	limiter := rate.NewLimiter(100.0/60, 10)
	current.Store(ip, limiter)
	atomic.AddInt64(&currentCount, 1)
	return limiter.Allow()
}

func rotate() {
	previous = current
	current = &sync.Map{}
	atomic.StoreInt64(&currentCount, 0)
}

package main

import (
	"net"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Rate limiters per IP with automatic cleanup
var (
	limiters sync.Map
	lastClean = time.Now()
)

func rateLimitAllow(addr string) bool {
	// Extract just the IP
	ip := addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		ip = host
	}
	
	// Clean old entries every hour
	if time.Since(lastClean) > time.Hour {
		lastClean = time.Now()
		limiters.Range(func(key, value interface{}) bool {
			if l, ok := value.(*rate.Limiter); ok && l.Tokens() >= 10 {
				limiters.Delete(key)
			}
			return true
		})
	}
	
	// Get or create limiter for this IP
	limiterInterface, _ := limiters.LoadOrStore(ip, rate.NewLimiter(100.0/60, 10))
	limiter := limiterInterface.(*rate.Limiter)
	
	return limiter.Allow()
}
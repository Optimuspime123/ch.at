package main

import (
	"net"
	"sync"
	"time"
)

// Simple in-memory rate limiter
// To disable: Remove NewRateLimiter calls from protocol files
type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
	limit    int           // requests per window
	window   time.Duration // time window
	stopCh   chan struct{} // for cleanup goroutine
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	r := &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	
	// Start cleanup goroutine
	go r.cleanup()
	
	return r
}

func (r *RateLimiter) Allow(addr string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Extract IP without port (addr might be "1.2.3.4:5678" or just "1.2.3.4")
	ip, _, _ := net.SplitHostPort(addr)
	if ip == "" {
		ip = addr // addr was already just an IP
	}

	now := time.Now()
	cutoff := now.Add(-r.window)

	// Get or create request list
	requests := r.requests[ip]
	
	// Remove old requests
	valid := []time.Time{}
	for _, t := range requests {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	// Check limit
	if len(valid) >= r.limit {
		return false
	}

	// Add new request
	valid = append(valid, now)
	r.requests[ip] = valid
	
	return true
}

// Periodic cleanup to prevent unbounded memory growth
func (r *RateLimiter) cleanup() {
	ticker := time.NewTicker(r.window)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			cutoff := now.Add(-r.window)
			
			// Remove IPs with no recent requests
			for ip, requests := range r.requests {
				valid := []time.Time{}
				for _, t := range requests {
					if t.After(cutoff) {
						valid = append(valid, t)
					}
				}
				
				if len(valid) == 0 {
					delete(r.requests, ip)
				} else {
					r.requests[ip] = valid
				}
			}
			r.mu.Unlock()
			
		case <-r.stopCh:
			return
		}
	}
}

// Stop the rate limiter cleanup
func (r *RateLimiter) Stop() {
	select {
	case <-r.stopCh:
		// Already closed
	default:
		close(r.stopCh)
	}
}

// Add other shared utilities here as needed
// Each should be self-contained and optional

// Global rate limiter instance
var rateLimiter = NewRateLimiter(100, time.Minute) // 100 requests per minute per IP
package api

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// ipRateLimiter holds per-IP rate limiters and cleans up stale entries.
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*ipEntry
	r       rate.Limit
	burst   int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	rl := &ipRateLimiter{
		entries: make(map[string]*ipEntry),
		r:       r,
		burst:   burst,
	}
	go rl.cleanup()
	return rl
}

func (rl *ipRateLimiter) get(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.entries[ip]
	if !ok {
		e = &ipEntry{limiter: rate.NewLimiter(rl.r, rl.burst)}
		rl.entries[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// cleanup removes IPs that haven't made a request in the last hour.
func (rl *ipRateLimiter) cleanup() {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for range t.C {
		rl.mu.Lock()
		for ip, e := range rl.entries {
			if time.Since(e.lastSeen) > time.Hour {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// rateLimitMiddleware returns a middleware that allows up to burst requests
// then r requests/second per unique client IP.
func rateLimitMiddleware(r rate.Limit, burst int) func(http.Handler) http.Handler {
	rl := newIPRateLimiter(r, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ip, _, err := net.SplitHostPort(req.RemoteAddr)
			if err != nil {
				ip = req.RemoteAddr
			}
			if !rl.get(ip).Allow() {
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please slow down")
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

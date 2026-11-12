package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/services"
)

// The forecast takes seconds to compute, and the dashboard asks for it on
// every visit. Results are cached per user and dropped whenever that user
// changes anything; the TTL bounds staleness from background changes (price
// moves, scheduled Plaid syncs and snapshots).
const forecastCacheTTL = 10 * time.Minute

type forecastCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]forecastCacheEntry
	// version counts invalidations per user, so a forecast computed while a
	// write was in flight isn't stored over the newer data.
	version map[uuid.UUID]uint64
}

type forecastCacheEntry struct {
	result    *services.ForecastResult
	expiresAt time.Time
}

func newForecastCache() *forecastCache {
	return &forecastCache{
		entries: make(map[uuid.UUID]forecastCacheEntry),
		version: make(map[uuid.UUID]uint64),
	}
}

// get returns a fresh cached result, or nil plus the version to pass to set.
func (c *forecastCache) get(userID uuid.UUID) (*services.ForecastResult, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.entries[userID]; ok && time.Now().Before(e.expiresAt) {
		return e.result, 0
	}
	return nil, c.version[userID]
}

func (c *forecastCache) set(userID uuid.UUID, version uint64, result *services.ForecastResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version[userID] != version {
		return
	}
	c.entries[userID] = forecastCacheEntry{result: result, expiresAt: time.Now().Add(forecastCacheTTL)}
}

func (c *forecastCache) invalidate(userID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, userID)
	c.version[userID]++
}

// invalidateForecastOnWrite drops the user's cached forecast around any
// non-GET request, since any of them may change forecast inputs.
func (s *Server) invalidateForecastOnWrite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		userID := userIDFromCtx(r)
		s.forecastCache.invalidate(userID)
		next.ServeHTTP(w, r)
		s.forecastCache.invalidate(userID)
	})
}

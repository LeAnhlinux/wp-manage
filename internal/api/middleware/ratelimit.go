package middleware

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RateLimiter implements an in-memory sliding window rate limiter for API key requests
type RateLimiter struct {
	mu      sync.RWMutex
	windows map[string]*slidingWindow
}

type slidingWindow struct {
	timestamps []time.Time
}

func NewRateLimiter() *RateLimiter {
	rl := &RateLimiter{
		windows: make(map[string]*slidingWindow),
	}
	// Background cleanup every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()
	return rl
}

// Handler returns a Fiber middleware that rate-limits API key requests
func (rl *RateLimiter) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Only rate-limit API key authenticated requests
		apiKeyID, ok := c.Locals("api_key_id").(uint64)
		if !ok || apiKeyID == 0 {
			return c.Next() // Not API key auth, skip rate limiting
		}

		rateLimit := 100
		if rl, ok := c.Locals("api_key_rate_limit").(int); ok && rl > 0 {
			rateLimit = rl
		}

		key := fmt.Sprintf("apikey:%d", apiKeyID)
		now := time.Now()
		windowStart := now.Add(-1 * time.Minute)

		rl.mu.Lock()
		w, exists := rl.windows[key]
		if !exists {
			w = &slidingWindow{}
			rl.windows[key] = w
		}

		// Remove expired timestamps
		valid := make([]time.Time, 0, len(w.timestamps))
		for _, ts := range w.timestamps {
			if ts.After(windowStart) {
				valid = append(valid, ts)
			}
		}
		w.timestamps = valid

		remaining := rateLimit - len(w.timestamps)
		if remaining <= 0 {
			rl.mu.Unlock()
			c.Set("X-RateLimit-Limit", strconv.Itoa(rateLimit))
			c.Set("X-RateLimit-Remaining", "0")
			c.Set("X-RateLimit-Reset", strconv.FormatInt(w.timestamps[0].Add(time.Minute).Unix(), 10))
			return c.Status(429).JSON(fiber.Map{
				"success": false,
				"error":   "Rate limit exceeded. Try again later.",
			})
		}

		w.timestamps = append(w.timestamps, now)
		rl.mu.Unlock()

		c.Set("X-RateLimit-Limit", strconv.Itoa(rateLimit))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining-1))

		return c.Next()
	}
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-2 * time.Minute)
	for key, w := range rl.windows {
		if len(w.timestamps) == 0 || w.timestamps[len(w.timestamps)-1].Before(cutoff) {
			delete(rl.windows, key)
		}
	}
}

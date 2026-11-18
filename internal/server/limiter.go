package server

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// limiter counts events per key (a client IP, a username) in a fixed window
// and reports how long a key that reached max has to wait. In memory: a
// restart forgets the counts, which only ever errs on the lenient side.
type limiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string]*count
}

type count struct {
	n     int
	reset time.Time
}

func newLimiter(max int, window time.Duration) *limiter {
	return &limiter{max: max, window: window, now: time.Now, hits: map[string]*count{}}
}

// wait returns how long key is blocked for, or 0.
func (l *limiter) wait(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.hits[key]
	if c == nil || c.n < l.max {
		return 0
	}
	d := c.reset.Sub(l.now())
	if d <= 0 {
		delete(l.hits, key)
		return 0
	}
	return d
}

// add counts one event for key.
func (l *limiter) add(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.hits) > 10000 { // don't let a flood of keys grow without bound
		for k, c := range l.hits {
			if !now.Before(c.reset) {
				delete(l.hits, k)
			}
		}
	}
	c := l.hits[key]
	if c == nil || !now.Before(c.reset) {
		c = &count{reset: now.Add(l.window)}
		l.hits[key] = c
	}
	c.n++
}

// clear forgets key (after a successful login).
func (l *limiter) clear(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.hits, key)
}

// limits are the server's limiters, created on first use.
type limits struct {
	loginIP   *limiter // failed logins per client IP
	loginUser *limiter // failed logins per username, from anywhere
	device    *limiter // SSO device-flow requests per client IP
}

func (s *Server) limits() *limits {
	s.limitOnce.Do(func() {
		s.lim = &limits{
			loginIP:   newLimiter(10, 15*time.Minute),
			loginUser: newLimiter(20, 15*time.Minute),
			device:    newLimiter(30, time.Minute),
		}
	})
	return s.lim
}

// tooMany answers 429 with Retry-After.
func tooMany(c *gin.Context, d time.Duration) {
	c.Header("Retry-After", strconv.Itoa(int(math.Ceil(d.Seconds()))))
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts, try again in " + d.Round(time.Second).String()})
}

// rateLimit counts every request per client IP against l.
func rateLimit(l func() *limiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		lim := l()
		if d := lim.wait(c.ClientIP()); d > 0 {
			tooMany(c, d)
			return
		}
		lim.add(c.ClientIP())
		c.Next()
	}
}

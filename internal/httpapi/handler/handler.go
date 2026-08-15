package handler

import (
	"net"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/benenen/lapin/internal/auth"
	"github.com/benenen/lapin/internal/identifier"
)

type Options struct {
	SecureCookies     bool
	TrustedProxyCIDRs []*net.IPNet
}

type Handler struct {
	db             *pgxpool.Pool
	ids            *identifier.Codec
	options        Options
	authLimiter    *fixedWindowLimiter
	apiLimiter     *fixedWindowLimiter
	openAPILimiter *fixedWindowLimiter
	accountLimiter *fixedWindowLimiter
	passwordWork   chan struct{}
	clientIP       app.ClientIP
}

var (
	dummyHashOnce sync.Once
	dummyHash     string
	dummyHashErr  error
)

func New(db *pgxpool.Pool, ids *identifier.Codec, options Options) *Handler {
	dummyHashOnce.Do(func() {
		dummyHash, dummyHashErr = auth.HashPassword("lapin-dummy-password-check")
	})
	if dummyHashErr != nil {
		panic(dummyHashErr)
	}
	return &Handler{
		db:             db,
		ids:            ids,
		options:        options,
		authLimiter:    newFixedWindowLimiter(10, time.Minute),
		apiLimiter:     newFixedWindowLimiter(300, time.Minute),
		openAPILimiter: newFixedWindowLimiter(60, time.Minute),
		accountLimiter: newFixedWindowLimiter(10, time.Minute),
		passwordWork:   make(chan struct{}, 2),
		clientIP: app.ClientIPWithOption(app.ClientIPOptions{
			RemoteIPHeaders: []string{"X-Forwarded-For", "X-Real-IP"},
			TrustedCIDRs:    options.TrustedProxyCIDRs,
		}),
	}
}

type rateWindow struct {
	startedAt time.Time
	count     int
}

type fixedWindowLimiter struct {
	mu        sync.Mutex
	windows   map[string]rateWindow
	limit     int
	window    time.Duration
	maxKeys   int
	lastSweep time.Time
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{windows: make(map[string]rateWindow), limit: limit, window: window, maxKeys: 10_000}
}

func (l *fixedWindowLimiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= l.window {
		for candidate, state := range l.windows {
			if now.Sub(state.startedAt) >= l.window {
				delete(l.windows, candidate)
			}
		}
		l.lastSweep = now
	}
	current, exists := l.windows[key]
	if !exists || now.Sub(current.startedAt) >= l.window {
		if !exists && len(l.windows) >= l.maxKeys {
			return false, l.window
		}
		l.windows[key] = rateWindow{startedAt: now, count: 1}
		return true, 0
	}
	if current.count >= l.limit {
		return false, l.window - now.Sub(current.startedAt)
	}
	current.count++
	l.windows[key] = current
	return true, 0
}

package httpserver

import (
	"context"
	"sync"
	"time"
)

type dynamicConcurrencyGate struct {
	mu           sync.Mutex
	active       int
	changed      chan struct{}
	pollInterval time.Duration
}

func newDynamicConcurrencyGate() *dynamicConcurrencyGate {
	return &dynamicConcurrencyGate{
		changed:      make(chan struct{}),
		pollInterval: 100 * time.Millisecond,
	}
}

func (g *dynamicConcurrencyGate) acquire(ctx context.Context, limit func() int) error {
	for {
		maxConcurrency := limit()
		if maxConcurrency < 1 {
			maxConcurrency = 1
		}

		g.mu.Lock()
		if g.active < maxConcurrency {
			g.active++
			g.mu.Unlock()
			return nil
		}
		changed := g.changed
		pollInterval := g.pollInterval
		g.mu.Unlock()

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return ctx.Err()
		case <-changed:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}
}

func (g *dynamicConcurrencyGate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active > 0 {
		g.active--
	}
	close(g.changed)
	g.changed = make(chan struct{})
}

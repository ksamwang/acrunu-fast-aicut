package localagent

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProcessorMediaLimiter(t *testing.T) {
	p := &processor{limiter: make(chan struct{}, localMediaProcessConcurrency)}
	started := make(chan struct{}, localMediaProcessConcurrency+1)
	release := make(chan struct{})
	done := make(chan struct{}, localMediaProcessConcurrency+1)

	for index := 0; index < localMediaProcessConcurrency+1; index++ {
		go func() {
			if err := p.acquire(context.Background()); err != nil {
				return
			}
			started <- struct{}{}
			<-release
			p.release()
			done <- struct{}{}
		}()
	}

	for index := 0; index < localMediaProcessConcurrency; index++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for media slot")
		}
	}
	select {
	case <-started:
		t.Fatalf("more than %d media operations started", localMediaProcessConcurrency)
	case <-time.After(100 * time.Millisecond):
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled acquire, got %v", err)
	}

	close(release)
	for index := 0; index < localMediaProcessConcurrency+1; index++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for media operation to finish")
		}
	}
}

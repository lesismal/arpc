package router

import (
	"errors"
	"sync"
	"time"

	"github.com/lesismal/arpc"
)

// ErrShutdown .
var ErrShutdown = errors.New("shutting down")

// Graceful represents a graceful middleware instance.
type Graceful struct {
	shutdown   bool
	gracefulWg sync.WaitGroup
}

// Handler returns the graceful middleware handler.
func (g *Graceful) Handler() arpc.HandlerFunc {
	return func(ctx *arpc.Context) {
		if !g.shutdown {
			g.gracefulWg.Add(1)
			defer g.gracefulWg.Done()
			ctx.Next()
		} else {
			ctx.Error(ErrShutdown)
		}
	}
}

// Shutdown stops handling new requests and waits for all current requests to be processed.
func (g *Graceful) Shutdown() {
	g.shutdown = true
	g.gracefulWg.Wait()
	time.Sleep(time.Second / 10)
}

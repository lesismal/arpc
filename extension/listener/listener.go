package listener

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/lesismal/arpc/log"
)

type event struct {
	err  error
	conn net.Conn
}

type Listener struct {
	logtag     string
	listener   net.Listener
	onlineA    int32
	onlineB    int32
	maxOnlineA int32
	chEventA   chan event
	chEventB   chan event
}

func (l *Listener) Accept() (net.Conn, error) {
	c, err := l.listener.Accept()
	return c, err
}

func (l *Listener) Close() error {
	if l.listener == nil {
		return nil
	}
	close(l.chEventA)
	close(l.chEventB)
	return l.listener.Close()
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	if l.listener == nil {
		return nil
	}
	return l.listener.Addr()
}

func (l *Listener) Listeners() (net.Listener, net.Listener) {
	return &ChanListener{
			addr:    l.listener.Addr(),
			chEvent: l.chEventA,
		}, &ChanListener{
			addr:    l.listener.Addr(),
			chEvent: l.chEventB,
		}
}

func (l *Listener) OfflineA() {
	atomic.AddInt32(&l.onlineA, -1)
}

func (l *Listener) Run() {
	tempSleep := time.Duration(0)
	maxSleep := time.Second
	for {
		c, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				log.Error("%v Accept error: %v; retrying...", l.logtag, err)
				if tempSleep == 0 {
					tempSleep = time.Millisecond * 20
				} else {
					tempSleep *= 2
				}
				if tempSleep > maxSleep {
					tempSleep = maxSleep
				}
				time.Sleep(tempSleep)
			} else {
				log.Error("%v Accept error: %v", l.logtag, err)
				return
			}
		} else {
			if atomic.AddInt32(&l.onlineA, 1) <= l.maxOnlineA {
				func() {
					// avoid conn leak when close
					defer func() {
						if err := recover(); err != nil {
							c.Close()
						}
					}()
					l.chEventA <- event{err: nil, conn: c}
				}()
			} else {
				atomic.AddInt32(&l.onlineA, -1)
				// avoid conn leak when close
				func() {
					defer func() {
						if err := recover(); err != nil {
							c.Close()
						}
					}()
					l.chEventB <- event{err: nil, conn: c}
				}()
			}
			tempSleep = 0
		}
	}
}

func Listen(network, addr string, maxOnlineA int, logtag string) (*Listener, error) {
	l, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}
	if maxOnlineA < 0 {
		maxOnlineA = 0
	}
	if logtag == "" {
		logtag = "[ARPC SVR]"
	}
	return &Listener{
		logtag:     logtag,
		listener:   l,
		onlineA:    0,
		onlineB:    0,
		maxOnlineA: int32(maxOnlineA),
		chEventA:   make(chan event, 4096),
		chEventB:   make(chan event, 4096),
	}, nil
}

type ChanListener struct {
	addr    net.Addr
	chEvent chan event
}

func (l *ChanListener) Accept() (net.Conn, error) {
	e, ok := <-l.chEvent
	if !ok {
		return nil, net.ErrClosed
	}
	return e.conn, e.err
}

func (l *ChanListener) Close() error {
	return nil
}

// Addr returns the listener's network address.
func (l *ChanListener) Addr() net.Addr {
	return l.addr
}

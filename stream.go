package arpc

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/lesismal/arpc/util"
)

// Stream .
type Stream struct {
	id            uint64
	cli           *Client
	method        string
	local         bool
	chData        chan *Message
	stateRecv     int32
	stateSend     int32
	stateCloseCnt int32
}

func (s *Stream) Id() uint64 {
	return s.id
}

func (s *Stream) isCreatedByLocal() bool {
	return s.local
}

func (s *Stream) onMessage(msg *Message) {
	if len(msg.Data()) == 0 {
		return
	}
	if atomic.LoadInt32(&s.stateRecv) == 1 {
		s.cli.Handler.OnMessageDone(s.cli, msg)
		return
	}
	if msg != nil {
		select {
		case s.chData <- msg:
		case <-s.cli.chClose:
		}
	}
}

func (s *Stream) CloseRecv() {
	if atomic.CompareAndSwapInt32(&s.stateRecv, 0, 1) {
		close(s.chData)
		s.close()
	}
}

func (s *Stream) CloseSend() {
	go s.CloseSendContext(context.Background())
}

func (s *Stream) CloseSendContext(ctx context.Context) {
	if atomic.CompareAndSwapInt32(&s.stateSend, 0, 1) {
		eof := true
		s.send(ctx, []byte{}, eof)
		s.close()
	}
}

func (s *Stream) Recv(v interface{}) error {
	if atomic.LoadInt32(&s.stateRecv) == 1 && len(s.chData) == 0 {
		return io.EOF
	}
	select {
	case msg, ok := <-s.chData:
		if !ok {
			return io.EOF
		}
		data := msg.Data()
		err := util.BytesToValue(s.cli.Codec, data, v)
		s.cli.Handler.OnMessageDone(s.cli, msg)
		return err
	case <-s.cli.chClose:
		return ErrClientStopped
	}
}

func (s *Stream) RecvContext(ctx context.Context, v interface{}) error {
	select {
	case msg := <-s.chData:
		data := msg.Data()
		err := util.BytesToValue(s.cli.Codec, data, v)
		s.cli.Handler.OnMessageDone(s.cli, msg)
		return err
	case <-ctx.Done():
		return ErrTimeout
	case <-s.cli.chClose:
		return ErrClientStopped
	}
}

func (s *Stream) RecvWith(ctx context.Context, v interface{}) error {
	return s.RecvContext(ctx, v)
}

func (s *Stream) newMessage(v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, nil)
	}
	return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, args[0].(map[interface{}]interface{}))
}

func (s *Stream) Send(v interface{}, args ...interface{}) error {
	return s.SendContext(context.Background(), v, args...)
}

func (s *Stream) SendContext(ctx context.Context, v interface{}, args ...interface{}) error {
	eof := false
	return s.checkStateAndSend(ctx, v, eof, args...)
}

func (s *Stream) SendWith(ctx context.Context, v interface{}, args ...interface{}) error {
	return s.SendContext(ctx, v, args...)
}

func (s *Stream) SendAndClose(v interface{}, args ...interface{}) error {
	return s.SendAndCloseContext(context.Background(), v, args...)
}

func (s *Stream) SendAndCloseContext(ctx context.Context, v interface{}, args ...interface{}) error {
	eof := true
	return s.checkStateAndSend(ctx, v, eof, args...)
}

func (s *Stream) SendAndCloseWith(ctx context.Context, v interface{}, args ...interface{}) error {
	return s.SendAndCloseContext(ctx, v, args...)
}

func (s *Stream) checkStateAndSend(ctx context.Context, v interface{}, eof bool, args ...interface{}) error {
	if atomic.LoadInt32(&s.stateSend) == 1 {
		return ErrStreamClosedSend
	}
	return s.send(ctx, v, eof, args...)
}

func (s *Stream) send(ctx context.Context, v interface{}, eof bool, args ...interface{}) error {
	c := s.cli
	err := c.CheckState()
	if err != nil {
		return err
	}

	data := util.ValueToBytes(c.Codec, v)
	msg := s.newMessage(data, args...)
	msg.SetStreamLocal(s.local)
	msg.SetStreamEOF(eof)

	if eof && atomic.CompareAndSwapInt32(&s.stateSend, 0, 1) {
		s.close()
	}

	if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
		case <-ctx.Done():
			return ErrTimeout
		}
	} else {
		if !c.reconnecting {
			coders := c.Handler.Coders()
			for j := 0; j < len(coders); j++ {
				msg = coders[j].Encode(c, msg)
			}
			_, err := c.Handler.Send(c.Conn, msg.Buffer)
			if err != nil {
				c.Conn.Close()
			}
			c.Handler.OnMessageDone(c, msg)
			return err
		} else {
			c.dropMessage(msg)
			return ErrClientReconnecting
		}
	}

	return nil
}

func (s *Stream) close() {
	// When cnt == 2, both recv and send are closed, then need to clear this stream
	if atomic.AddInt32(&s.stateCloseCnt, 1) == 2 {
		s.cli.deleteStream(s.id, s.local)
		// n := len(s.chData)
		// for i := 0; i < n; i++ {
		// 	select {
		// 	case msg := <-s.chData:
		// 		s.cli.Handler.OnMessageDone(s.cli, msg)
		// 	default:
		// 		return
		// 	}
		// }
	}
}

// NewStream creates a stream.
func (client *Client) NewStream(method string) *Stream {
	return client.newStream(method, 0, true)
}

func (client *Client) newStream(method string, id uint64, local bool) *Stream {
	if id == 0 {
		id = atomic.AddUint64(&client.seq, 1)
	}
	stream := &Stream{
		id:     id,
		cli:    client,
		method: method,
		chData: make(chan *Message, client.Handler.StreamQueueSize()),
		local:  local,
	}
	client.mux.Lock()
	if local {
		client.streamLocalMap[stream.id] = stream
	} else {
		client.streamRemoteMap[stream.id] = stream
	}
	client.mux.Unlock()
	return stream
}

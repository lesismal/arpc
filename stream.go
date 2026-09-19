package arpc

import (
	"context"
	"io"
	"sync/atomic"

	"github.com/lesismal/arpc/util"
)

// Stream is a bidirectional message stream over a Client, identified by id
// and bound to a method. Each side can close its send and receive halves
// separately; the Stream is removed from the Client once both are closed.
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

// Id returns the stream id.
func (s *Stream) Id() uint64 {
	return s.id
}

// onMessage queues an incoming message for Recv. Messages with an empty body
// are ignored, and messages arriving after CloseRecv are released.
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

// CloseRecv closes the receive half. Messages already queued can still be read
// by Recv, after which it returns io.EOF.
func (s *Stream) CloseRecv() {
	if atomic.CompareAndSwapInt32(&s.stateRecv, 0, 1) {
		close(s.chData)
		s.halfClose()
	}
}

// CloseRecvContext is the same as CloseRecv; ctx is unused.
func (s *Stream) CloseRecvContext(ctx context.Context) {
	s.CloseRecv()
}

// CloseSend closes the send half asynchronously, see CloseSendContext.
func (s *Stream) CloseSend() {
	go s.CloseSendContext(context.Background())
}

// CloseSendContext closes the send half by sending an EOF message to the peer.
// Only the first call has effect; ctx bounds the time waiting for the send
// queue.
func (s *Stream) CloseSendContext(ctx context.Context) {
	if atomic.CompareAndSwapInt32(&s.stateSend, 0, 1) {
		eof := true
		_ = s.send(ctx, []byte{}, eof)
		s.halfClose()
	}
}

// Recv waits for the next message and decodes it into v, see
// util.BytesToValue. It returns io.EOF after the receive half is closed and
// drained, or ErrClientStopped if the Client stops.
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

// RecvContext is like Recv, and returns ErrTimeout when ctx is done.
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

// RecvWith is an alias of RecvContext.
func (s *Stream) RecvWith(ctx context.Context, v interface{}) error {
	return s.RecvContext(ctx, v)
}

// newMessage builds a CmdStream message; args[0], if any, must be a
// map[interface{}]interface{} used as the message values.
func (s *Stream) newMessage(v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, nil)
	}
	return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, args[0].(map[interface{}]interface{}))
}

// Send sends v to the peer, see SendContext.
func (s *Stream) Send(v interface{}, args ...interface{}) error {
	return s.SendContext(context.Background(), v, args...)
}

// SendContext sends v to the peer. args[0], if any, must be a
// map[interface{}]interface{} of message values. With AsyncWrite, ctx bounds
// the time waiting for the send queue. It returns ErrStreamClosedSend after
// the send half is closed.
func (s *Stream) SendContext(ctx context.Context, v interface{}, args ...interface{}) error {
	eof := false
	return s.checkStateAndSend(ctx, v, eof, args...)
}

// SendWith is an alias of SendContext.
func (s *Stream) SendWith(ctx context.Context, v interface{}, args ...interface{}) error {
	return s.SendContext(ctx, v, args...)
}

// SendAndClose sends v and closes the send half, see SendAndCloseContext.
func (s *Stream) SendAndClose(v interface{}, args ...interface{}) error {
	return s.SendAndCloseContext(context.Background(), v, args...)
}

// SendAndCloseContext is like SendContext, and marks the message as EOF so
// the send half is closed with it.
func (s *Stream) SendAndCloseContext(ctx context.Context, v interface{}, args ...interface{}) error {
	eof := true
	return s.checkStateAndSend(ctx, v, eof, args...)
}

// SendAndCloseWith is an alias of SendAndCloseContext.
func (s *Stream) SendAndCloseWith(ctx context.Context, v interface{}, args ...interface{}) error {
	return s.SendAndCloseContext(ctx, v, args...)
}

func (s *Stream) checkStateAndSend(ctx context.Context, v interface{}, eof bool, args ...interface{}) error {
	if atomic.LoadInt32(&s.stateSend) == 1 {
		return ErrStreamClosedSend
	}
	return s.send(ctx, v, eof, args...)
}

// send encodes v into a stream message and sends it in the Client's write
// mode: the writev queue, the send queue, or directly on the conn. An EOF
// message also closes the send half.
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
		s.halfClose()
	}

	if c.Handler.AsyncWritev() {
		return c.pushWritev(msg)
	} else if c.Handler.AsyncWrite() {
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

// halfClose counts a closed half, and removes the Stream from the Client once
// both halves are closed.
func (s *Stream) halfClose() {
	if atomic.AddInt32(&s.stateCloseCnt, 1) == 2 {
		s.cli.deleteStream(s.id, s.local)
	}
}

// NewStream creates a local Stream for method. The peer's handler for method,
// registered by Handler.HandleStream, is called on its first message.
func (client *Client) NewStream(method string) *Stream {
	return client.newStream(method, 0, true)
}

// newStream creates a Stream and registers it on the Client. id 0 allocates a
// new id; local tells whether it was created on this side or by the peer.
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

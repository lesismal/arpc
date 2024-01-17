package arpc

import (
	"encoding/binary"
	"io"
	"sync/atomic"

	"github.com/lesismal/arpc/util"
)

// stream .
type Stream struct {
	id          uint64
	cli         *Client
	method      string
	chData      chan *Message
	stateDone   int32
	stateClosed int32
	local       bool
}

func (s *Stream) isCreatedByLocal() bool {
	return s.local
}

func (s *Stream) onMessage(msg *Message) {
	if msg != nil {
		select {
		case s.chData <- msg:
		case <-s.cli.chClose:
		}
	}
}

func (s *Stream) done() {
	if atomic.CompareAndSwapInt32(&s.stateDone, 0, 1) {
		select {
		case s.chData <- nil:
		case <-s.cli.chClose:
		}
	}
}

func (s *Stream) Recv(v interface{}) error {
	select {
	case msg, ok := <-s.chData:
		if !ok {
			return io.EOF
		}
		data := msg.Data()
		if len(data) == 0 {
			s.cli.Handler.OnMessageDone(s.cli, msg)
			return io.EOF
		}
		err := s.cli.Codec.Unmarshal(data, v)
		s.cli.Handler.OnMessageDone(s.cli, msg)
		return err
	case <-s.cli.chClose:
		return ErrClientStopped
	}
}

// func (s *Stream) RecvWith(ctx context.Context, v interface{}) error {
// 	return s.RecvContext(ctx, v)
// }

// func (s *Stream) RecvContext(ctx context.Context, v interface{}) error {
// 	var (
// 		ok   bool
// 		data []byte
// 	)
// 	select {
// 	case data, ok = <-s.chData:
// 		if !ok || len(data) == 0 {
// 			return io.EOF
// 		}
// 	case <-ctx.Done():
// 		return ErrTimeout
// 	}
// 	return s.cli.Codec.Unmarshal(data, v)
// }

func (s *Stream) newMessage(v interface{}, args ...interface{}) *Message {
	if len(args) == 0 {
		return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, nil)
	}
	return newMessage(CmdStream, s.method, v, false, false, s.id, s.cli.Handler, s.cli.Codec, args[0].(map[interface{}]interface{}))
}

func (s *Stream) send(v interface{}, done bool, args ...interface{}) error {
	c := s.cli
	err := c.CheckState()
	if err != nil {
		return err
	}

	data := util.ValueToBytes(c.Codec, v)
	buf := c.Handler.Malloc(8 + len(data))
	binary.LittleEndian.PutUint64(buf, s.id)
	copy(buf[8:], data)
	msg := s.newMessage(buf, args...)
	msg.SetStreamLocal(s.local)
	msg.SetStreamDone(done)
	c.Handler.Free(buf)

	if c.Handler.AsyncWrite() {
		select {
		case c.chSend <- msg:
		case <-c.chClose:
			// c.Handler.OnOverstock(c, msg)
			c.Handler.OnMessageDone(c, msg)
			return ErrClientStopped
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

// func (s *Stream) SendWith(ctx context.Context, v interface{}, timeout time.Duration, args ...interface{}) error {
// 	return s.SendContext(ctx, v, args...)
// }

// func (s *Stream) SendContext(ctx context.Context, v interface{}, args ...interface{}) error {
// 	c := s.cli
// 	err := c.CheckState()
// 	if err != nil {
// 		return err
// 	}

// 	data := util.ValueToBytes(c.Codec, v)
// 	buf := c.Handler.Malloc(8 + len(data))
// 	binary.LittleEndian.PutUint64(buf, s.id)
// 	copy(buf[8:], data)
// 	msg := c.newRequestMessage(CmdStream, s.method, buf, false, false, args...)
// 	c.Handler.Free(buf)

// 	if c.Handler.AsyncWrite() {
// 		select {
// 		case c.chSend <- msg:
// 		case <-ctx.Done():
// 			// c.Handler.OnOverstock(c, msg)
// 			c.Handler.OnMessageDone(c, msg)
// 			return ErrClientTimeout
// 		case <-c.chClose:
// 			// c.Handler.OnOverstock(c, msg)
// 			c.Handler.OnMessageDone(c, msg)
// 			return ErrClientStopped
// 		}
// 	} else {
// 		if !c.reconnecting {
// 			coders := c.Handler.Coders()
// 			for j := 0; j < len(coders); j++ {
// 				msg = coders[j].Encode(c, msg)
// 			}
// 			_, err := c.Handler.Send(c.Conn, msg.Buffer)
// 			if err != nil {
// 				c.Conn.Close()
// 			}
// 			c.Handler.OnMessageDone(c, msg)
// 			return err
// 		} else {
// 			c.dropMessage(msg)
// 			return ErrClientReconnecting
// 		}
// 	}

// 	return err
// }

func (s *Stream) Close(args ...interface{}) error {
	var err error
	if atomic.CompareAndSwapInt32(&s.stateClosed, 0, 1) {
		defer s.done()
		err = s.send([]byte{}, true, args...)
		s.cli.deleteStream(s.id, s.local)
		n := len(s.chData)
		for i := 0; i < n; i++ {
			select {
			case msg := <-s.chData:
				s.cli.Handler.OnMessageDone(s.cli, msg)
			default:
				goto Exit
			}
		}
	}
Exit:
	return err
}

func (s *Stream) Send(v interface{}, args ...interface{}) error {
	return s.send(v, false, args...)
}

func (s *Stream) SendAndClose(v interface{}, args ...interface{}) error {
	return s.send(v, true, args...)
}

// NewStream creates a stream.
func (client *Client) NewStream(method string) *Stream {
	return client.newStream(method, true)
}

func (client *Client) newStream(method string, local bool) *Stream {
	stream := &Stream{
		id:     atomic.AddUint64(&client.seq, 1),
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

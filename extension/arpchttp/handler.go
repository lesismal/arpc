package arpchttp

import (
	"io"
	"net"
	"net/http"
	"time"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
)

type Conn struct {
	w http.ResponseWriter
}

func (c *Conn) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (c *Conn) Write(b []byte) (n int, err error) {
	n, err = c.w.Write(b)
	return n, err
}

func (c *Conn) Close() error {
	return nil
}

func (c *Conn) LocalAddr() net.Addr {
	return nil
}

func (c *Conn) RemoteAddr() net.Addr {
	return nil
}

func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

func Handler(nh arpc.Handler, codec codec.Codec) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		client := &arpc.Client{Conn: &Conn{w}, Codec: codec, Handler: nh}
		msg := nh.NewMessageWithBuffer(body)
		nh.OnMessage(client, msg)
	}
}

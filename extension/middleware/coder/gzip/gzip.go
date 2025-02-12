package gzip

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/extension/middleware/coder"
)

func gzipCompress(data []byte) []byte {
	var in bytes.Buffer
	w := gzip.NewWriter(&in)
	w.Write(data)
	w.Close()
	return in.Bytes()
}

func gzipUnCompress(data []byte) ([]byte, error) {
	b := bytes.NewReader(data)
	r, err := gzip.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	undatas, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return undatas, nil
}

// Gzip represents a gzip coding middleware.
type Gzip int

// Encode implements arpc MessageCoder.
func (g *Gzip) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if len(*msg.PBuffer) > int(*g) && !msg.IsFlagBitSet(coder.FlagBitGZip) {
		buf := gzipCompress((*msg.PBuffer)[arpc.HeaderIndexReserved+1:])
		total := len(buf) + arpc.HeaderIndexReserved + 1
		if total < len(*msg.PBuffer) {
			copy((*msg.PBuffer)[arpc.HeaderIndexReserved+1:], buf)
			*msg.PBuffer = (*msg.PBuffer)[:total]
			msg.SetBodyLen(total - 16)
			msg.SetFlagBit(coder.FlagBitGZip, true)
		}
	}
	return msg
}

// Decode implements arpc MessageCoder.
func (g *Gzip) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if msg.IsFlagBitSet(coder.FlagBitGZip) {
		buf, err := gzipUnCompress((*msg.PBuffer)[arpc.HeaderIndexReserved+1:])
		if err == nil {
			*msg.PBuffer = (*msg.PBuffer)[:arpc.HeaderIndexReserved+1]
			msg.PBuffer = msg.Handler().Append(msg.PBuffer, buf...)
			msg.SetFlagBit(coder.FlagBitGZip, false)
			msg.SetBodyLen(len(*msg.PBuffer) - 16)
		}
	}
	return msg
}

// New returns the gzip coding middleware.
func New(n int) *Gzip {
	var g = Gzip(n)
	return &g
}

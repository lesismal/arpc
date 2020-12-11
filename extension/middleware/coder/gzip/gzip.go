package gzip

import (
	"bytes"
	"compress/gzip"
	"io/ioutil"

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
	undatas, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return undatas, nil
}

// Gzip represents a gzip coding middleware.
type Gzip struct {
	critical int
}

// Encode implements arpc MessageCoder.
func (c *Gzip) Encode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if len(msg.Buffer) > c.critical && !msg.IsFlagBitSet(coder.FlagBitGZip) {
		buf := gzipCompress(msg.Buffer[arpc.HeaderIndexReserved+1:])
		total := len(buf) + arpc.HeaderIndexReserved + 1
		if total < len(msg.Buffer) {
			copy(msg.Buffer[arpc.HeaderIndexReserved+1:], buf)
			msg.Buffer = msg.Buffer[:total]
			msg.SetBodyLen(total - 16)
			msg.SetFlagBit(coder.FlagBitGZip, true)
		}
	}
	return msg
}

// Decode implements arpc MessageCoder.
func (c *Gzip) Decode(client *arpc.Client, msg *arpc.Message) *arpc.Message {
	if msg.IsFlagBitSet(coder.FlagBitGZip) {
		buf, err := gzipUnCompress(msg.Buffer[arpc.HeaderIndexReserved+1:])
		if err == nil {
			msg.Buffer = append(msg.Buffer[:arpc.HeaderIndexReserved+1], buf...)
			msg.SetFlagBit(coder.FlagBitGZip, false)
			msg.SetBodyLen(len(msg.Buffer) - 16)
		}
	}
	return msg
}

// New returns the gzip coding middleware.
func New() *Gzip {
	return &Gzip{critical: 1024}
}

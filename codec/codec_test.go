// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"encoding/gob"
	"testing"
)

type payload struct {
	A int
	B string
}

func TestJSONCodec(t *testing.T) {
	c := &JSONCodec{}
	data, err := c.Marshal(&payload{A: 1, B: "b"})
	if err != nil || string(data) != `{"A":1,"B":"b"}` {
		t.Fatalf("Marshal = %s, %v", data, err)
	}
	var p payload
	if err := c.Unmarshal(data, &p); err != nil || p != (payload{A: 1, B: "b"}) {
		t.Fatalf("Unmarshal = %+v, %v", p, err)
	}
	if _, err := c.Marshal(make(chan int)); err == nil {
		t.Fatal("Marshal of an unsupported value should fail")
	}
	if err := c.Unmarshal([]byte("{"), &p); err == nil {
		t.Fatal("Unmarshal of bad data should fail")
	}
}

// gobCodec is a Codec other than the default one.
type gobCodec struct{}

func (gobCodec) Marshal(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(v)
	return buf.Bytes(), err
}

func (gobCodec) Unmarshal(data []byte, v interface{}) error {
	return gob.NewDecoder(bytes.NewReader(data)).Decode(v)
}

func TestSetCodec(t *testing.T) {
	old := DefaultCodec
	defer SetCodec(old)

	SetCodec(gobCodec{})
	if _, ok := DefaultCodec.(gobCodec); !ok {
		t.Fatal("SetCodec not applied")
	}
	data, err := DefaultCodec.Marshal(&payload{A: 1, B: "b"})
	if err != nil {
		t.Fatal(err)
	}
	var p payload
	if err := DefaultCodec.Unmarshal(data, &p); err != nil || p != (payload{A: 1, B: "b"}) {
		t.Fatalf("round trip = %+v, %v", p, err)
	}
}

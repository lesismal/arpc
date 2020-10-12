// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package codec

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
)

// codecGob .
type codecGob struct{}

// Marshal .
func (c *codecGob) Marshal(v interface{}) ([]byte, error) {
	buffer := &bytes.Buffer{}
	err := gob.NewEncoder(buffer).Encode(v)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// Unmarshal .
func (c *codecGob) Unmarshal(data []byte, v interface{}) error {
	return gob.NewDecoder(bytes.NewBuffer(data)).Decode(v)
}
func TestJSONCodec_Marshal(t *testing.T) {
	type Value struct {
		I int
		S string
	}

	v1 := &Value{I: 3, S: "hello"}
	v2 := &Value{}
	jc := &JSONCodec{}
	data, err := jc.Marshal(v1)
	if err != nil {
		t.Errorf("JSONCodec.Marshal() error = %v, wantErr %v", err, nil)
		return
	}
	err = jc.Unmarshal(data, v2)
	if err != nil {
		t.Errorf("JSONCodec.Unmarshal() error = %v, wantErr %v", err, nil)
		return
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Errorf("v2 = %v, want %v", v2, v1)
	}
}

func TestJSONCodec_Unmarshal(t *testing.T) {
	TestJSONCodec_Marshal(t)
}

func TestSetCodec(t *testing.T) {
	type Value struct {
		I int
		S string
	}

	gc := &codecGob{}
	SetCodec(gc)

	v1 := &Value{I: 3, S: "hello"}
	v2 := &Value{}
	dc := DefaultCodec
	data, err := dc.Marshal(v1)
	if err != nil {
		t.Errorf("JSONCodec.Marshal() error = %v, wantErr %v", err, nil)
		return
	}
	err = gc.Unmarshal(data, v2)
	if err != nil {
		t.Errorf("JSONCodec.Unmarshal() error = %v, wantErr %v", err, nil)
		return
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Errorf("v2 = %v, want %v", v2, v1)
	}
}

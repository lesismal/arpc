// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package codec

import (
	"encoding/json"
)

// DefaultCodec is the codec used by arpc when none is specified.
var DefaultCodec Codec = &JSONCodec{}

// Codec encodes and decodes the body of arpc Messages.
//
// Marshal returns the encoding of v.
//
// Unmarshal decodes data and stores the result in the value pointed to by v.
type Codec interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
}

// JSONCodec is a Codec based on encoding/json.
type JSONCodec struct{}

// Marshal calls json.Marshal.
func (j *JSONCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal calls json.Unmarshal.
func (j *JSONCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// SetCodec replaces DefaultCodec.
func SetCodec(c Codec) {
	DefaultCodec = c
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package codec

import (
	"encoding/json"
)

// DefaultCodec instance
var DefaultCodec Codec = &JSONCodec{} // jsoniter.ConfigCompatibleWithStandardLibrary

// Codec definition
type Codec interface {
	Marshal(v interface{}) ([]byte, error)
	Unmarshal(data []byte, v interface{}) error
}

// JSONCodec definition
type JSONCodec struct{}

// Marshal implements Codec
func (j *JSONCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal implements Codec
func (j *JSONCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// SetCodec sets default codec instance
func SetCodec(c Codec) {
	DefaultCodec = c
}

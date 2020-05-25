// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"reflect"
	"testing"
)

func TestContext_Body(t *testing.T) {
	tests := []struct {
		name string
		ctx  *Context
		want []byte
	}{
		struct {
			name string
			ctx  *Context
			want []byte
		}{
			name: "normal body",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}),
			},
			want: []byte{1, 2, 3, 4, 5, 6, 7, 8},
		},
		struct {
			name string
			ctx  *Context
			want []byte
		}{
			name: "ref message, null body",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8}).cloneHead(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.Body(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Context.Body() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContext_Bind(t *testing.T) {
	type args struct {
		v interface{}
	}
	tests := []struct {
		name    string
		ctx     *Context
		args    args
		wantErr bool
	}{
		struct {
			name    string
			ctx     *Context
			args    args
			wantErr bool
		}{
			name: "bind cloned message",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}).cloneHead(),
			},
			args:    args{},
			wantErr: true,
		},
		struct {
			name    string
			ctx     *Context
			args    args
			wantErr bool
		}{
			name: "bind error message",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{1, 0, 1, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'a', 'b', 'c', 'd'}),
			},
			args:    args{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.Bind(tt.args.v); (err != nil) != tt.wantErr {
				t.Errorf("Context.Bind() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContext_Write(t *testing.T) {
	type args struct {
		v interface{}
	}
	tests := []struct {
		name    string
		ctx     *Context
		args    args
		wantErr bool
	}{
		struct {
			name    string
			ctx     *Context
			args    args
			wantErr bool
		}{
			name: "should only response to a request message",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
			args:    args{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.Write(tt.args.v); (err == ErrShouldOnlyResponseToRequestMessage) != tt.wantErr {
				t.Errorf("Context.Write() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContext_Clone(t *testing.T) {
	tests := []struct {
		name string
		ctx  *Context
		want *Context
	}{
		struct {
			name string
			ctx  *Context
			want *Context
		}{
			name: "clone Context",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
			want: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{refFlagByte, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.Clone(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Context.Clone() = %v, want %v", got, tt.want)
			}
		})
	}
}

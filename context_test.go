// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import (
	"reflect"
	"testing"
	"time"
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
		name string
		ctx  *Context
		args args
		want bool
	}{
		struct {
			name string
			ctx  *Context
			args args
			want bool
		}{
			name: "bind error message",
			ctx: &Context{
				Client:  &Client{Codec: DefaultCodec},
				Message: Message([]byte{1, 0, 1, 0, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'a', 'b', 'c', 'd'}),
			},
			args: args{},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.Bind(tt.args.v); (err != nil) != tt.want {
				t.Errorf("Context.Bind() error = %v, want %v", err, tt.want)
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

func TestContext_WriteWithTimeout(t *testing.T) {
	type args struct {
		v       interface{}
		timeout time.Duration
	}
	tests := []struct {
		name    string
		ctx     *Context
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.WriteWithTimeout(tt.args.v, tt.args.timeout); (err != nil) != tt.wantErr {
				t.Errorf("Context.WriteWithTimeout() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContext_Error(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name    string
		ctx     *Context
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.Error(tt.args.err); (err != nil) != tt.wantErr {
				t.Errorf("Context.Error() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestContext_newRspMessage(t *testing.T) {
	type args struct {
		v       interface{}
		isError byte
	}
	tests := []struct {
		name string
		ctx  *Context
		args args
		want Message
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ctx.newRspMessage(tt.args.v, tt.args.isError); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Context.newRspMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContext_write(t *testing.T) {
	type args struct {
		v       interface{}
		isError byte
		timeout time.Duration
	}
	tests := []struct {
		name    string
		ctx     *Context
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.ctx.write(tt.args.v, tt.args.isError, tt.args.timeout); (err != nil) != tt.wantErr {
				t.Errorf("Context.write() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_newContext(t *testing.T) {
	type args struct {
		c   *Client
		msg Message
	}
	tests := []struct {
		name string
		args args
		want *Context
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newContext(tt.args.c, tt.args.msg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

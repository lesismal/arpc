// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "errors"

// client error
var (
	// ErrClientTimeout
	ErrClientTimeout = errors.New("timeout")
	// ErrClientOverstock
	ErrClientOverstock = errors.New("timeout: rpc client's send queue is full")
	// ErrClientReconnecting
	ErrClientReconnecting = errors.New("client reconnecting")
	// ErrClientStopped
	ErrClientStopped = errors.New("client stopped")
)

// message error
var (
	// ErrInvalidRspMessage
	ErrInvalidRspMessage = errors.New("invalid response message cmd")

	// ErrMethodNotFound .
	ErrMethodNotFound = errors.New("method not found")
)

// context error
var (
	// ErrContextErrWritten .
	ErrContextErrWritten = errors.New("context erorr written")

	// ErrContextResponseToNotify .
	ErrContextResponseToNotify = errors.New("should not response to context with notify message")
)

// general errors
var (
	// ErrTimeout
	ErrTimeout = errors.New("timeout")
)

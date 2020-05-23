// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "errors"

// client error
var (
	// ErrClientTimeout
	ErrClientTimeout = errors.New("timeout")
	// ErrClientQueueIsFull
	ErrClientQueueIsFull = errors.New("timeout: rpc client's send queue is full")
	// ErrClientReconnecting
	ErrClientReconnecting = errors.New("client reconnecting")
	// ErrClientStopped
	ErrClientStopped = errors.New("client stopped")
)

// message error
var (
	// ErrInvalidBodyLen
	ErrInvalidBodyLen = errors.New("invalid body length")
	// ErrInvalidMessage
	ErrInvalidMessage = errors.New("invalid message")
	// ErrInvalidMessageMethod
	ErrInvalidMessageMethod = errors.New("invalid message method")
	// ErrInvalidRspMessage
	ErrInvalidRspMessage = errors.New("invalid response message cmd")
)

// context error
var (
	// ErrBindClonedContex
	ErrBindClonedContex = errors.New("invalid operation: bind a cloned Context, should only bind before Context.Clone to avoid more memory cost")

	// ErrResponseToResponsedMessage
	ErrResponseToResponsedMessage = errors.New("invalid operation: reply to a responsed message, should only reply to a request message")
)

// general errors
var (
	// ErrTimeout
	ErrTimeout = errors.New("timeout")

	// ErrUnexpected
	ErrUnexpected = errors.New("unexpected error")
)

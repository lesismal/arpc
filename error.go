// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "errors"

// client error
var (
	// ErrClientTimeout .
	ErrClientTimeout = errors.New("timeout")

	// ErrClientInvalidTimeoutZero .
	ErrClientInvalidTimeoutZero = errors.New("invalid timeout, should > 0")

	// ErrClientInvalidTimeoutLessThanZero .
	ErrClientInvalidTimeoutLessThanZero = errors.New("invalid timeout, should >= 0")

	// ErrClientInvalidTimeoutZeroWithNonNilHandler .
	ErrClientInvalidTimeoutZeroWithNonNilHandler = errors.New("invalid timeout 0 with non-nil async handler")

	// ErrClientOverstock .
	ErrClientOverstock = errors.New("timeout: rpc client's send queue is full")
	// ErrClientReconnecting .
	ErrClientReconnecting = errors.New("client reconnecting")
	// ErrClientStopped .
	ErrClientStopped = errors.New("client stopped")

	// ErrClientInvalidPoolDialers .
	ErrClientInvalidPoolDialers = errors.New("invalid dialers array")
)

// message error
var (
	// ErrInvalidRspMessage .
	ErrInvalidRspMessage = errors.New("invalid response message cmd")

	// ErrMethodNotFound .
	ErrMethodNotFound = errors.New("method not found")

	// ErrInvalidFlagBitIndex .
	ErrInvalidFlagBitIndex = errors.New("invalid index, should use[0~9]")
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
	// ErrTimeout .
	ErrTimeout = errors.New("timeout")
)

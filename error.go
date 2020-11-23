// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "errors"

// client error
var (
	// ErrClientTimeout represents a timeout error because of timer or context.
	ErrClientTimeout = errors.New("timeout")

	// ErrClientInvalidTimeoutZero represents an error of 0 time parameter.
	ErrClientInvalidTimeoutZero = errors.New("invalid timeout, should not be 0")

	// ErrClientInvalidTimeoutLessThanZero represents an error of less than 0 time parameter.
	ErrClientInvalidTimeoutLessThanZero = errors.New("invalid timeout, should not be < 0")

	// ErrClientInvalidTimeoutZeroWithNonNilCallback represents an error with 0 time parameter but with non-nil callback.
	ErrClientInvalidTimeoutZeroWithNonNilCallback = errors.New("invalid timeout 0 with non-nil callback")

	// ErrClientOverstock represents an error of Client's send queue is full.
	ErrClientOverstock = errors.New("timeout: rpc Client's send queue is full")

	// ErrClientReconnecting represents an error that Client is reconnecting.
	ErrClientReconnecting = errors.New("client reconnecting")

	// ErrClientStopped represents an error that Client is stopped.
	ErrClientStopped = errors.New("client stopped")

	// ErrClientInvalidPoolDialers represents an error of empty dialer array.
	ErrClientInvalidPoolDialers = errors.New("invalid dialers: empty array")
)

// message error
var (
	// ErrInvalidRspMessage represents an error of invalid message CMD.
	ErrInvalidRspMessage = errors.New("invalid response message cmd")

	// ErrMethodNotFound represents an error of method not found.
	ErrMethodNotFound = errors.New("method not found")

	// ErrInvalidFlagBitIndex represents an error of invlaid flag bit index.
	ErrInvalidFlagBitIndex = errors.New("invalid index, should be 0-7")
)

// context error
var (
	// ErrContextResponseToNotify represents an error that response to a notify message.
	ErrContextResponseToNotify = errors.New("should not response to a context with notify message")
)

// general errors
var (
	// ErrTimeout represents an error of timeout.
	ErrTimeout = errors.New("timeout")
)

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "errors"

// client errors
var (
	// ErrClientTimeout is returned when a call times out or its context is done.
	ErrClientTimeout = errors.New("timeout")

	// ErrClientInvalidTimeoutZero is returned when a timeout of 0 is not allowed.
	ErrClientInvalidTimeoutZero = errors.New("invalid timeout, should not be 0")

	// ErrClientInvalidTimeoutLessThanZero is returned for a negative timeout.
	ErrClientInvalidTimeoutLessThanZero = errors.New("invalid timeout, should not be < 0")

	// ErrClientInvalidTimeoutZeroWithNonNilCallback is reserved for a timeout of 0
	// with a non-nil callback. It is currently not returned by arpc.
	ErrClientInvalidTimeoutZeroWithNonNilCallback = errors.New("invalid timeout 0 with non-nil callback")

	// ErrClientOverstock is returned when the Client's send queue stays full
	// until the timeout.
	ErrClientOverstock = errors.New("timeout: rpc Client's send queue is full")

	// ErrClientReconnecting is returned when the Client is reconnecting.
	ErrClientReconnecting = errors.New("client reconnecting")

	// ErrClientStopped is returned when the Client has been stopped.
	ErrClientStopped = errors.New("client stopped")

	// ErrClientInvalidPoolDialers is returned by NewClientPoolFromDialers when
	// no dialer is given.
	ErrClientInvalidPoolDialers = errors.New("invalid dialers: empty array")

	// ErrClientInvalidAsyncHandler is returned when the async handler is nil.
	ErrClientInvalidAsyncHandler = errors.New("invalid async handler: should not be nil")
)

// message errors
var (
	// ErrInvalidRspMessage is returned when a response message's Cmd is not
	// CmdResponse.
	ErrInvalidRspMessage = errors.New("invalid response message cmd")

	// ErrMethodNotFound is sent back to the caller when no handler is registered
	// for the requested method.
	ErrMethodNotFound = errors.New("method not found")

	// ErrInvalidFlagBitIndex is returned when a flag bit index is out of 0-7.
	ErrInvalidFlagBitIndex = errors.New("invalid index, should be 0-7")
)

// context errors
var (
	// ErrContextResponseToNotify is returned when writing a response to a
	// Notify message, which expects none.
	ErrContextResponseToNotify = errors.New("should not response to a context with notify message")
)

// stream errors
var (
	// ErrStreamClosedSend is returned when sending on a Stream whose send side
	// has been closed.
	ErrStreamClosedSend = errors.New("stream has closed send")
)

// general errors
var (
	// ErrTimeout is returned when an operation times out.
	ErrTimeout = errors.New("timeout")
)

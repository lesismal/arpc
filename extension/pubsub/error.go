// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package pubsub

import "errors"

var (
	// ErrInvalidPassword .
	ErrInvalidPassword = errors.New("invalid password")

	// ErrInvalidTopicEmpty .
	ErrInvalidTopicEmpty = errors.New("invalid topic, should not be \"\"")

	// ErrInvalidTopicBytes .
	ErrInvalidTopicBytes = errors.New("invalid topic bytes, should be more than 10 bytes")

	// ErrInvalidTopicNameLength .
	ErrInvalidTopicNameLength = errors.New("invalid topic name length, should not be more than 1024")
)

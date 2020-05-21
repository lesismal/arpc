// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package arpc

import "log"

// DefaultLogger instance
var DefaultLogger Logger = &StdLogger{Enable: true}

// Logger defines log interface
type Logger interface {
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

// SetLogger set default logger for arpc
func SetLogger(l Logger) {
	DefaultLogger = l
}

type StdLogger struct {
	Enable bool
}

func (l *StdLogger) Debug(format string, v ...interface{}) {
	if l.Enable {
		log.Printf(format, v...)
	}
}

func (l *StdLogger) Info(format string, v ...interface{}) {
	if l.Enable {
		log.Printf(format, v...)
	}
}

func (l *StdLogger) Warn(format string, v ...interface{}) {
	if l.Enable {
		log.Printf(format, v...)
	}
}

func (l *StdLogger) Error(format string, v ...interface{}) {
	if l.Enable {
		log.Printf(format, v...)
	}
}

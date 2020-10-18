// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package log

import (
	"log"
)

// DefaultLogger instance
var DefaultLogger Logger = &logger{level: LogLevelInfo}

const (
	// LogLevelAll .
	LogLevelAll = iota
	// LogLevelDebug .
	LogLevelDebug
	// LogLevelInfo .
	LogLevelInfo
	// LogLevelWarn .
	LogLevelWarn
	// LogLevelError .
	LogLevelError
	// LogLevelNone .
	LogLevelNone
)

// Logger defines log interface
type Logger interface {
	SetLogLevel(lvl int)
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

// SetLogger set default logger for arpc
func SetLogger(l Logger) {
	DefaultLogger = l
}

// SetLogLevel .
func SetLogLevel(lvl int) {
	switch lvl {
	case LogLevelAll, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelNone:
		DefaultLogger.SetLogLevel(lvl)
		break
	default:
		log.Printf("invalid log level: %v", lvl)
	}
}

// logger defines default logger
type logger struct {
	level int
}

// SetLogLevel .
func (l *logger) SetLogLevel(lvl int) {
	switch lvl {
	case LogLevelAll, LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError, LogLevelNone:
		l.level = lvl
		break
	default:
		log.Printf("invalid log level: %v", lvl)
	}
}

// Debug .
func (l *logger) Debug(format string, v ...interface{}) {
	if LogLevelDebug >= l.level {
		log.Printf("[DBG] "+format, v...)
	}
}

// Info .
func (l *logger) Info(format string, v ...interface{}) {
	if LogLevelInfo >= l.level {
		log.Printf("[INF] "+format, v...)
	}
}

// Warn .
func (l *logger) Warn(format string, v ...interface{}) {
	if LogLevelWarn >= l.level {
		log.Printf("[WRN] "+format, v...)
	}
}

// Error .
func (l *logger) Error(format string, v ...interface{}) {
	if LogLevelError >= l.level {
		log.Printf("[Err] "+format, v...)
	}
}

// Debug .
func Debug(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Debug(format, v...)
	}
}

// Info .
func Info(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Info(format, v...)
	}
}

// Warn .
func Warn(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Warn(format, v...)
	}
}

// Error .
func Error(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Error(format, v...)
	}
}

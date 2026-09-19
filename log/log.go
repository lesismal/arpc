// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package log

import (
	"fmt"
	"io"
	"os"
	"time"
)

var (
	// TimeFormat is the timestamp layout of each log line.
	TimeFormat = "2006/01/02 15:04:05.000"

	// Output is where the default logger writes to.
	Output io.Writer = os.Stdout

	// DefaultLogger is the logger used by arpc, at LevelInfo by default.
	DefaultLogger Logger = &logger{level: LevelInfo}
)

const (
	// LevelAll enables all logs.
	LevelAll = iota
	// LevelDebug is for debug logs, usually disabled in production.
	LevelDebug
	// LevelInfo is the default level.
	LevelInfo
	// LevelWarn is for warnings.
	LevelWarn
	// LevelError is for errors.
	LevelError
	// LevelNone disables all logs.
	LevelNone
)

// Logger is the logging interface used by arpc.
type Logger interface {
	SetLevel(lvl int)
	Debug(format string, v ...interface{})
	Info(format string, v ...interface{})
	Warn(format string, v ...interface{})
	Error(format string, v ...interface{})
}

// SetLogger replaces DefaultLogger.
func SetLogger(l Logger) {
	DefaultLogger = l
}

// SetLevel sets the level of DefaultLogger. An invalid level is reported to
// Output and ignored.
func SetLevel(lvl int) {
	switch lvl {
	case LevelAll, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelNone:
		DefaultLogger.SetLevel(lvl)
		break
	default:
		fmt.Fprintf(Output, "invalid log level: %v", lvl)
	}
}

// logger is the default Logger implementation, writing to Output.
type logger struct {
	level int
}

// SetLevel sets the minimum level to log. An invalid level is ignored.
func (l *logger) SetLevel(lvl int) {
	switch lvl {
	case LevelAll, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelNone:
		l.level = lvl
		break
	default:
		fmt.Fprintf(Output, "invalid log level: %v", lvl)
	}
}

// Debug writes a message to Output at LevelDebug.
func (l *logger) Debug(format string, v ...interface{}) {
	if LevelDebug >= l.level {
		fmt.Fprintf(Output, time.Now().Format(TimeFormat)+" [DBG] "+format+"\n", v...)
	}
}

// Info writes a message to Output at LevelInfo.
func (l *logger) Info(format string, v ...interface{}) {
	if LevelInfo >= l.level {
		fmt.Fprintf(Output, time.Now().Format(TimeFormat)+" [INF] "+format+"\n", v...)
	}
}

// Warn writes a message to Output at LevelWarn.
func (l *logger) Warn(format string, v ...interface{}) {
	if LevelWarn >= l.level {
		fmt.Fprintf(Output, time.Now().Format(TimeFormat)+" [WRN] "+format+"\n", v...)
	}
}

// Error writes a message to Output at LevelError.
func (l *logger) Error(format string, v ...interface{}) {
	if LevelError >= l.level {
		fmt.Fprintf(Output, time.Now().Format(TimeFormat)+" [ERR] "+format+"\n", v...)
	}
}

// Debug logs a message at LevelDebug via DefaultLogger, if it is not nil.
func Debug(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Debug(format, v...)
	}
}

// Info logs a message at LevelInfo via DefaultLogger, if it is not nil.
func Info(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Info(format, v...)
	}
}

// Warn logs a message at LevelWarn via DefaultLogger, if it is not nil.
func Warn(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Warn(format, v...)
	}
}

// Error logs a message at LevelError via DefaultLogger, if it is not nil.
func Error(format string, v ...interface{}) {
	if DefaultLogger != nil {
		DefaultLogger.Error(format, v...)
	}
}

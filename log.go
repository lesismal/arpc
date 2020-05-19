package arpc

import "log"

// DefaultLogger instance
var DefaultLogger Logger = &logger{}

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

type logger struct{}

func (l *logger) Debug(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (l *logger) Info(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (l *logger) Warn(format string, v ...interface{}) {
	log.Printf(format, v...)
}

func (l *logger) Error(format string, v ...interface{}) {
	log.Printf(format, v...)
}

// Copyright 2020 lesismal. All rights reserved.
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package log

import "testing"

func TestSetLogger(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	SetLogger(l)
}

func TestSetLogLevel(t *testing.T) {
	SetLogLevel(LogLevelAll)
	func() {
		defer func() { recover() }()
		SetLogLevel(1000)
	}()
}

func Test_logger_SetLogLevel(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	l.SetLogLevel(LogLevelAll)
}

func Test_logger_Debug(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	l.Debug("logger debug test")
}

func Test_logger_Info(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	l.Info("logger info test")
}

func Test_logger_Warn(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	l.Warn("logger warn test")
}

func Test_logger_Error(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	l.Error("logger error test")
}

func Test_Debug(t *testing.T) {
	Debug("log.Debug")
}

func Test_Info(t *testing.T) {
	Info("log.Info")
}

func Test_Warn(t *testing.T) {
	Warn("log.Warn")
}

func Test_Error(t *testing.T) {
	Error("log.Error")
}

package arpc

import "testing"

func TestSetLogger(t *testing.T) {
	l := &logger{level: LogLevelDebug}
	SetLogger(l)
}

func TestSetLogLevel(t *testing.T) {
	SetLogLevel(LogLevelAll)
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

func Test_logDebug(t *testing.T) {
	logDebug("logDebug")
}

func Test_logInfo(t *testing.T) {
	logInfo("logInfo")
}

func Test_logWarn(t *testing.T) {
	logWarn("logWarn")
}

func Test_logError(t *testing.T) {
	logError("logError")
}

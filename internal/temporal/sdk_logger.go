package temporal

import (
	"fmt"

	"gestalt/internal/logging"
)

type sdkLogger struct {
	logger *logging.Logger
}

func newSDKLogger(logger *logging.Logger) *sdkLogger {
	return &sdkLogger{logger: logger}
}

// Debug intentionally no-ops to keep Temporal SDK logs at INFO+.
func (l *sdkLogger) Debug(msg string, keyvals ...interface{}) {}

func (l *sdkLogger) Info(msg string, keyvals ...interface{}) {
	l.log(logging.LevelInfo, msg, keyvals...)
}

func (l *sdkLogger) Warn(msg string, keyvals ...interface{}) {
	l.log(logging.LevelWarning, msg, keyvals...)
}

func (l *sdkLogger) Error(msg string, keyvals ...interface{}) {
	l.log(logging.LevelError, msg, keyvals...)
}

func (l *sdkLogger) log(level logging.Level, message string, keyvals ...interface{}) {
	if l == nil || l.logger == nil {
		return
	}
	fields := map[string]string{
		"gestalt.category": "workflow",
		"gestalt.source":   "temporal-sdk",
	}
	for i := 0; i+1 < len(keyvals); i += 2 {
		key := fmt.Sprint(keyvals[i])
		fields[key] = fmt.Sprint(keyvals[i+1])
	}
	switch level {
	case logging.LevelWarning:
		l.logger.Warn(message, fields)
	case logging.LevelError:
		l.logger.Error(message, fields)
	default:
		l.logger.Info(message, fields)
	}
}

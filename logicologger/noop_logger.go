package logicologger

import (
	"context"
)

type NoopLogger struct{}

func (l *NoopLogger) Id() string {
	return "noop-logger"
}

func (l *NoopLogger) Debug(format string, args ...interface{}) {
}

func (l *NoopLogger) Info(format string, args ...interface{}) {
}

func (l *NoopLogger) Warn(format string, args ...interface{}) {
}

func (l *NoopLogger) Error(format string, args ...interface{}) {
}

func (l *NoopLogger) Log(level LogLevel, format string, args ...interface{}) {
}

func (l *NoopLogger) Start(ctx context.Context) error {
	return nil
}

func (l *NoopLogger) Stop() error {
	return nil
}

func (l *NoopLogger) Prefix() string {
	return ""
}

func (l *NoopLogger) SetPrefix(prefix string) {
}

func NewNoopLogger() Logger {
	return &NoopLogger{}
}

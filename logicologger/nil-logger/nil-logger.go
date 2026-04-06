package nil_logger

import (
	"context"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type NilLogger struct {
}

func (n *NilLogger) Level() logicologger.LogLevel {
	return 0
}

func (n *NilLogger) SetLevel(level logicologger.LogLevel) {
	return
}

func (n *NilLogger) AddChild(logger logicologger.ParentLogger, s string, s2 string, m map[string]interface{}) logicologger.ParentLogger {
	return n
}

func (n *NilLogger) OnChildClose(logger logicologger.Logger, i []interface{}) {
	return
}

func (n *NilLogger) Id() string {
	return "nil"
}

func (n *NilLogger) Debug(s string, i ...interface{}) {
	return
}

func (n *NilLogger) Info(s string, i ...interface{}) {
	return
}

func (n *NilLogger) Warn(s string, i ...interface{}) {
	return
}

func (n *NilLogger) Error(s string, i ...interface{}) {
	return
}

func (n *NilLogger) Log(level logicologger.LogLevel, s string, i ...interface{}) {
	return
}

func (n *NilLogger) Start(ctx context.Context) error {
	return nil
}

func (n *NilLogger) Stop() error {
	return nil
}

func (n *NilLogger) Prefix() string {
	return ""
}

func (n *NilLogger) SetPrefix(s string) {
	return
}

func NewNilLogger() logicologger.Logger {
	return &NilLogger{}
}

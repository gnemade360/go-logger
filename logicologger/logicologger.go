package logicologger

import (
	"context"
	"io"
	"time"
)

type ParentLogger interface {
	Level() LogLevel
	SetLevel(level LogLevel)
	AddChild(ParentLogger, string, string, map[string]interface{}) ParentLogger
	OnChildClose(Logger, []interface{})
	Logger
}
type LogWriter interface {
	io.WriteCloser
}
type LogLevel int

const (
	ERROR LogLevel = 1 << iota
	INFO
	DEBUG
	WARN
)

type (
	LogData map[string]interface{}

	LogEvent struct {
		Prefix  string
		Message string
		Time    time.Time
		Data    LogData
		Bytes   []byte
		Level   LogLevel
		Indent  int
	}

	Logger interface {
		Id() string
		Debug(string, ...interface{})
		Info(string, ...interface{})
		Warn(string, ...interface{})
		Error(string, ...interface{})
		Log(LogLevel, string, ...interface{})
		Start(context.Context) error
		Stop() error
		Prefix() string
		SetPrefix(string)
	}
	LogParser interface {
		Parse(entry LogEvent) (LogEvent, error)
	}
)

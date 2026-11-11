package general_logger_test

import (
	"context"
	"testing"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
	general_logger "github.com/gnemade360/go-logger/logicologger/general-logger"
)

type messageFormatter struct{}

func (messageFormatter) Format(event logicologger.LogEvent) ([]byte, error) {
	return []byte(event.Message), nil
}

type captureWriter struct {
	lines chan string
}

func newCaptureWriter() *captureWriter {
	return &captureWriter{lines: make(chan string, 10)}
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.lines <- string(p)
	return len(p), nil
}

func (w *captureWriter) Close() error {
	return nil
}

func (w *captureWriter) next(t *testing.T) (string, bool) {
	t.Helper()
	select {
	case line := <-w.lines:
		return line, true
	case <-time.After(time.Second):
		return "", false
	}
}

func newTestLogger(t *testing.T, w *captureWriter, opts ...general_logger.GeneralLoggerOption) *general_logger.GeneralLogger {
	t.Helper()
	opts = append([]general_logger.GeneralLoggerOption{
		general_logger.WithWriter(w),
		general_logger.WithFormatter(messageFormatter{}),
	}, opts...)
	logger := general_logger.NewGeneralLogger(context.Background(), "test", nil, opts...)
	t.Cleanup(func() { logger.Stop() })
	return logger
}

func TestGeneralLogger_FormatsMessageWithArgs(t *testing.T) {
	tests := []struct {
		name string
		log  func(logicologger.Logger)
		want string
	}{
		{
			name: "info with args",
			log:  func(l logicologger.Logger) { l.Info("starting: %s on port %d", "app", 8080) },
			want: "starting: app on port 8080",
		},
		{
			name: "error with args",
			log:  func(l logicologger.Logger) { l.Error("failed: %v", "boom") },
			want: "failed: boom",
		},
		{
			name: "no args keeps literal percent",
			log:  func(l logicologger.Logger) { l.Info("100% done") },
			want: "100% done",
		},
		{
			name: "trailing context func is not a format arg",
			log: func(l logicologger.Logger) {
				l.Info("user %s", "bob", func() interface{} { return "ctx" })
			},
			want: "user bob",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newCaptureWriter()
			logger := newTestLogger(t, w)

			tt.log(logger)

			got, ok := w.next(t)
			if !ok {
				t.Fatalf("no log line written, want %q", tt.want)
			}
			if got != tt.want {
				t.Errorf("message got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGeneralLogger_KeepsArgsInOtherData(t *testing.T) {
	events := make(chan logicologger.LogEvent, 1)
	logger := newTestLogger(t, newCaptureWriter())
	logger.OnLog = func(event logicologger.LogEvent) { events <- event }

	logger.Info("user %s", "bob")

	select {
	case event := <-events:
		other, _ := event.Data["other"].([]interface{})
		if len(other) != 1 || other[0] != "bob" {
			t.Errorf("Data[other] got = %v, want [bob]", event.Data["other"])
		}
	case <-time.After(time.Second):
		t.Fatal("no log event received")
	}
}

func TestGeneralLogger_DefaultLevel(t *testing.T) {
	tests := []struct {
		name      string
		log       func(logicologger.Logger)
		wantWrite bool
	}{
		{name: "error", log: func(l logicologger.Logger) { l.Error("e") }, wantWrite: true},
		{name: "warn", log: func(l logicologger.Logger) { l.Warn("w") }, wantWrite: true},
		{name: "info", log: func(l logicologger.Logger) { l.Info("i") }, wantWrite: true},
		{name: "debug", log: func(l logicologger.Logger) { l.Debug("d") }, wantWrite: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newCaptureWriter()
			logger := newTestLogger(t, w)

			tt.log(logger)

			_, written := w.next(t)
			if written != tt.wantWrite {
				t.Errorf("written got = %v, want %v", written, tt.wantWrite)
			}
		})
	}
}

func TestGeneralLogger_WithLogLevelOverridesDefault(t *testing.T) {
	w := newCaptureWriter()
	logger := newTestLogger(t, w, general_logger.WithLogLevel(logicologger.DEBUG))

	logger.Info("dropped")
	logger.Debug("kept")

	got, ok := w.next(t)
	if !ok || got != "kept" {
		t.Errorf("got = %q (written %v), want %q", got, ok, "kept")
	}
}

func TestGeneralLogger_Id(t *testing.T) {
	logger := newTestLogger(t, newCaptureWriter())

	if got := logger.Id(); got != "test" {
		t.Errorf("Id() got = %q, want %q", got, "test")
	}

	child := logger.AddChild(nil, "child", "", nil)
	t.Cleanup(func() { child.Stop() })
	if got := child.Id(); got != "test-_-child" {
		t.Errorf("child Id() got = %q, want %q", got, "test-_-child")
	}
}

package general_logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
	general_logger "github.com/gnemade360/go-logger/logicologger/general-logger"
	file_log_writer "github.com/gnemade360/go-logger/logicologger/log-writers/file-log-writer"
	map_log_writer "github.com/gnemade360/go-logger/logicologger/log-writers/map-log-writer"
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

func TestGeneralLogger_StoresOnlyArgsTheMessageDidNotUse(t *testing.T) {
	tests := []struct {
		name      string
		log       func(logicologger.Logger)
		wantMsg   string
		wantOther []interface{}
	}{
		{
			name:    "formatted args are not stored",
			log:     func(l logicologger.Logger) { l.Info("user %s", "bob") },
			wantMsg: "user bob",
		},
		{
			name:      "args without format verbs are stored",
			log:       func(l logicologger.Logger) { l.Info("plain", "x") },
			wantMsg:   "plain",
			wantOther: []interface{}{"x"},
		},
		{
			name:    "escaped percent is not a verb",
			log:     func(l logicologger.Logger) { l.Info("100%% of %d", 3) },
			wantMsg: "100% of 3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan logicologger.LogEvent, 1)
			logger := newTestLogger(t, newCaptureWriter())
			logger.OnLog = func(event logicologger.LogEvent) { events <- event }

			tt.log(logger)

			select {
			case event := <-events:
				if event.Message != tt.wantMsg {
					t.Errorf("Message got = %q, want %q", event.Message, tt.wantMsg)
				}
				other, hasOther := event.Data["other"]
				if tt.wantOther == nil && hasOther {
					t.Errorf("Data[other] got = %v, want no key", other)
				}
				if tt.wantOther != nil && !reflect.DeepEqual(other, tt.wantOther) {
					t.Errorf("Data[other] got = %v, want %v", other, tt.wantOther)
				}
			case <-time.After(time.Second):
				t.Fatal("no log event received")
			}
		})
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

type orderWriter struct {
	mu         sync.Mutex
	got        []int
	inFlight   atomic.Int32
	concurrent atomic.Bool
	received   atomic.Int64
}

func (w *orderWriter) Write(p []byte) (int, error) {
	if w.inFlight.Add(1) > 1 {
		w.concurrent.Store(true)
	}
	defer w.inFlight.Add(-1)
	n, err := strconv.Atoi(string(p))
	if err != nil {
		return 0, err
	}
	if n%100 == 0 {
		time.Sleep(time.Millisecond)
	}
	w.mu.Lock()
	w.got = append(w.got, n)
	w.mu.Unlock()
	w.received.Add(1)
	return len(p), nil
}

func (w *orderWriter) Close() error { return nil }

func TestGeneralLogger_WritesInOrderOneAtATime(t *testing.T) {
	const total = 10000
	const maxAhead = 500
	w := &orderWriter{}
	logger := general_logger.NewGeneralLogger(context.Background(), "order", nil,
		general_logger.WithWriter(w), general_logger.WithFormatter(messageFormatter{}))
	t.Cleanup(func() { logger.Stop() })

	for i := 0; i < total; i++ {
		for int64(i)-w.received.Load() > maxAhead {
			time.Sleep(50 * time.Microsecond)
		}
		logger.Info(strconv.Itoa(i))
	}
	deadline := time.Now().Add(10 * time.Second)
	for w.received.Load() < total && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if w.concurrent.Load() {
		t.Error("writer was called concurrently")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.got) != total {
		t.Fatalf("events written got = %d, want %d", len(w.got), total)
	}
	for i, n := range w.got {
		if n != i {
			t.Fatalf("event at position %d got = %d, want %d", i, n, i)
		}
	}
}

type bufferWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *bufferWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *bufferWriter) Close() error { return nil }

func (w *bufferWriter) waitForLines(n int) []byte {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		w.mu.Lock()
		hasAll := bytes.Count(w.buf.Bytes(), []byte("\n")) >= n
		out := append([]byte(nil), w.buf.Bytes()...)
		w.mu.Unlock()
		if hasAll {
			return out
		}
		time.Sleep(5 * time.Millisecond)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

func TestGeneralLogger_DefaultOutputIsOneJSONObjectPerLine(t *testing.T) {
	w := &bufferWriter{}
	logger := general_logger.NewGeneralLogger(context.Background(), "lines", nil, general_logger.WithWriter(w))
	t.Cleanup(func() { logger.Stop() })

	logger.Info("first")
	logger.Info("second")
	out := w.waitForLines(2)

	lines := bytes.Split(bytes.TrimSuffix(out, []byte("\n")), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("lines got = %d, want 2, output %q", len(lines), out)
	}
	mapWriter := map_log_writer.NewMapLogWriter()
	for i, line := range lines {
		if !json.Valid(line) {
			t.Errorf("line %d is not valid JSON: %q", i, line)
		}
		if _, err := mapWriter.Write(append(line, '\n')); err != nil {
			t.Errorf("map writer rejected line %d: %v", i, err)
		}
	}
	path := filepath.Join(t.TempDir(), "log.ndjson")
	fileWriter, err := file_log_writer.NewFileLogWriter(path)
	if err != nil {
		t.Fatalf("NewFileLogWriter() error = %v", err)
	}
	if _, err := fileWriter.Write(out); err != nil {
		t.Fatalf("file writer Write() error = %v", err)
	}
	fileWriter.Close()
	written, _ := os.ReadFile(path)
	if got := bytes.Count(written, []byte("\n")); got != 2 {
		t.Errorf("file lines got = %d, want 2", got)
	}
}

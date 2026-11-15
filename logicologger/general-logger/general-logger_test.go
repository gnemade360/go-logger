package general_logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestGeneralLogger_ConcurrentLogSetLevelAndCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	logger := general_logger.NewGeneralLogger(ctx, "race", nil,
		general_logger.WithWriter(&bufferWriter{}), general_logger.WithFormatter(messageFormatter{}))
	t.Cleanup(func() { logger.Stop() })

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 2000 {
				logger.Info("event")
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			logger.SetLevel(logicologger.INFO | logicologger.DEBUG)
			_ = logger.Level()
		}
	}()
	time.Sleep(time.Millisecond)
	cancel()
	wg.Wait()
}

type closeRecorder struct {
	delay   time.Duration
	release chan struct{}
	written atomic.Int64
	atClose atomic.Int64
	closed  chan struct{}
	once    sync.Once
}

func newCloseRecorder(delay time.Duration) *closeRecorder {
	w := &closeRecorder{delay: delay, closed: make(chan struct{})}
	w.atClose.Store(-1)
	return w
}

func (w *closeRecorder) Write(p []byte) (int, error) {
	if w.release != nil {
		<-w.release
	}
	time.Sleep(w.delay)
	w.written.Add(1)
	return len(p), nil
}

// Close records only the first call: GeneralWriter's finalizer closes the targets again later.
func (w *closeRecorder) Close() error {
	w.once.Do(func() {
		w.atClose.Store(w.written.Load())
		close(w.closed)
	})
	return nil
}

func newRecordedLogger(t *testing.T, ctx context.Context, w *closeRecorder, opts ...general_logger.GeneralLoggerOption) *general_logger.GeneralLogger {
	t.Helper()
	opts = append([]general_logger.GeneralLoggerOption{
		general_logger.WithWriter(w),
		general_logger.WithFormatter(messageFormatter{}),
	}, opts...)
	return general_logger.NewGeneralLogger(ctx, "flush", nil, opts...)
}

func TestGeneralLogger_StopWritesQueuedEventsBeforeClose(t *testing.T) {
	const total = 500
	w := newCloseRecorder(100 * time.Microsecond)
	logger := newRecordedLogger(t, context.Background(), w)

	for i := range total {
		logger.Info(strconv.Itoa(i))
	}
	logger.Stop()

	if got := w.atClose.Load(); got != total {
		t.Errorf("events written before Close got = %d, want %d", got, total)
	}
}

func TestGeneralLogger_ContextCancelFlushesLikeStop(t *testing.T) {
	const total = 100
	ctx, cancel := context.WithCancel(context.Background())
	w := newCloseRecorder(100 * time.Microsecond)
	logger := newRecordedLogger(t, ctx, w)
	t.Cleanup(func() { logger.Stop() })

	for i := range total {
		logger.Info(strconv.Itoa(i))
	}
	cancel()

	select {
	case <-w.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("writer not closed after context cancel")
	}
	if got := w.atClose.Load(); got != total {
		t.Errorf("events written before Close got = %d, want %d", got, total)
	}
}

func TestGeneralLogger_FlushWritesQueuedEvents(t *testing.T) {
	const total = 50
	w := newCloseRecorder(200 * time.Microsecond)
	logger := newRecordedLogger(t, context.Background(), w)
	t.Cleanup(func() { logger.Stop() })

	for i := range total {
		logger.Info(strconv.Itoa(i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := logger.Flush(ctx)

	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if got := w.written.Load(); got != total {
		t.Errorf("events written after Flush got = %d, want %d", got, total)
	}
}

func TestGeneralLogger_FlushHonoursContext(t *testing.T) {
	w := newCloseRecorder(0)
	w.release = make(chan struct{})
	logger := newRecordedLogger(t, context.Background(), w, general_logger.WithStopTimeout(10*time.Millisecond))
	t.Cleanup(func() { close(w.release); logger.Stop() })

	logger.Info("stuck")
	logger.Info("queued")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := logger.Flush(ctx)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Flush() error got = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestGeneralLogger_StopReturnsWithinTimeoutWhenWriterHangs(t *testing.T) {
	w := newCloseRecorder(0)
	w.release = make(chan struct{})
	t.Cleanup(func() { close(w.release) })
	logger := newRecordedLogger(t, context.Background(), w, general_logger.WithStopTimeout(100*time.Millisecond))

	logger.Info("hangs")
	started := time.Now()
	err := logger.Stop()
	elapsed := time.Since(started)

	if elapsed > time.Second {
		t.Errorf("Stop() took %v, want about 100ms", elapsed)
	}
	if err == nil {
		t.Error("Stop() error got = nil, want a timeout error")
	}
}

type gateWriter struct {
	entered     chan struct{}
	enteredOnce sync.Once
	release     chan struct{}
	mu          sync.Mutex
	lines       []string
}

func newGateWriter() *gateWriter {
	return &gateWriter{entered: make(chan struct{}), release: make(chan struct{})}
}

func (w *gateWriter) Write(p []byte) (int, error) {
	w.enteredOnce.Do(func() { close(w.entered) })
	<-w.release
	w.mu.Lock()
	w.lines = append(w.lines, string(p))
	w.mu.Unlock()
	return len(p), nil
}

func (w *gateWriter) Close() error { return nil }

func (w *gateWriter) written() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func newGatedLogger(t *testing.T, w *gateWriter, opts ...general_logger.GeneralLoggerOption) *general_logger.GeneralLogger {
	t.Helper()
	opts = append([]general_logger.GeneralLoggerOption{
		general_logger.WithWriter(w),
		general_logger.WithFormatter(messageFormatter{}),
	}, opts...)
	logger := general_logger.NewGeneralLogger(context.Background(), "overflow", nil, opts...)
	t.Cleanup(func() { logger.Stop() })
	return logger
}

func waitEntered(t *testing.T, w *gateWriter) {
	t.Helper()
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("writer was never called")
	}
}

func flushWithin(t *testing.T, logger *general_logger.GeneralLogger) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := logger.Flush(ctx); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
}

func TestGeneralLogger_ReportsDroppedEventsInTheLog(t *testing.T) {
	const defaultQueueSize = 1000
	w := newGateWriter()
	logger := newGatedLogger(t, w)

	logger.Info("first")
	waitEntered(t, w)
	for i := range defaultQueueSize + 5 {
		logger.Info(strconv.Itoa(i))
	}
	close(w.release)
	flushWithin(t, logger)

	lines := w.written()
	warnings := 0
	for _, line := range lines {
		if line == "dropped 5 log events" {
			warnings++
		}
	}
	if warnings != 1 {
		t.Errorf("dropped warnings got = %d, want 1, lines written %d", warnings, len(lines))
	}
}

func TestGeneralLogger_OverflowDropCountsAndWarnsBeforeNextEvent(t *testing.T) {
	w := newGateWriter()
	logger := newGatedLogger(t, w, general_logger.WithQueueSize(1))

	logger.Info("e1")
	waitEntered(t, w)
	logger.Info("e2")
	logger.Info("e3")
	logger.Info("e4")
	logger.Info("e5")

	if got := logger.Dropped(); got != 3 {
		t.Errorf("Dropped() got = %d, want 3", got)
	}
	close(w.release)
	flushWithin(t, logger)
	want := []string{"e1", "dropped 3 log events", "e2"}
	if got := w.written(); !reflect.DeepEqual(got, want) {
		t.Errorf("lines got = %q, want %q", got, want)
	}
}

func TestGeneralLogger_OverflowBlockLosesNothing(t *testing.T) {
	const total = 5
	w := newGateWriter()
	logger := newGatedLogger(t, w, general_logger.WithQueueSize(1), general_logger.WithOverflow(general_logger.OverflowBlock))

	logged := make(chan struct{})
	go func() {
		defer close(logged)
		for i := range total {
			logger.Info(strconv.Itoa(i))
		}
	}()
	waitEntered(t, w)
	select {
	case <-logged:
		t.Fatal("Log returned while the queue was full, want it to wait")
	case <-time.After(50 * time.Millisecond):
	}
	close(w.release)
	<-logged
	flushWithin(t, logger)

	want := []string{"0", "1", "2", "3", "4"}
	if got := w.written(); !reflect.DeepEqual(got, want) {
		t.Errorf("lines got = %q, want %q", got, want)
	}
	if got := logger.Dropped(); got != 0 {
		t.Errorf("Dropped() got = %d, want 0", got)
	}
}

func TestGeneralLogger_StopReleasesBlockedLoggers(t *testing.T) {
	w := newGateWriter()
	logger := newGatedLogger(t, w, general_logger.WithQueueSize(1),
		general_logger.WithOverflow(general_logger.OverflowBlock), general_logger.WithStopTimeout(50*time.Millisecond))
	t.Cleanup(func() { close(w.release) })

	logger.Info("held by the writer")
	waitEntered(t, w)
	logger.Info("fills the queue")
	logged := make(chan struct{})
	go func() {
		defer close(logged)
		logger.Info("waits for room")
	}()
	logger.Stop()

	select {
	case <-logged:
	case <-time.After(time.Second):
		t.Fatal("Log still blocked after Stop")
	}
}

type closeGuardWriter struct {
	delay            time.Duration
	inWrite          atomic.Bool
	closed           atomic.Bool
	closeDuringWrite atomic.Bool
	writeAfterClose  atomic.Bool
}

func (w *closeGuardWriter) Write(p []byte) (int, error) {
	if w.closed.Load() {
		w.writeAfterClose.Store(true)
	}
	w.inWrite.Store(true)
	time.Sleep(w.delay)
	w.inWrite.Store(false)
	return len(p), nil
}

func (w *closeGuardWriter) Close() error {
	if w.inWrite.Load() {
		w.closeDuringWrite.Store(true)
	}
	w.closed.Store(true)
	return nil
}

func TestGeneralLogger_StopTimeoutNeverClosesUnderAWrite(t *testing.T) {
	w := &closeGuardWriter{delay: 30 * time.Millisecond}
	logger := general_logger.NewGeneralLogger(context.Background(), "guard", nil,
		general_logger.WithWriter(w), general_logger.WithFormatter(messageFormatter{}),
		general_logger.WithStopTimeout(10*time.Millisecond))

	logger.Info("a")
	logger.Info("b")
	logger.Info("c")
	err := logger.Stop()
	deadline := time.Now().Add(time.Second)
	for !w.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}

	if err == nil {
		t.Error("Stop() error got = nil, want a timeout error")
	}
	if !w.closed.Load() {
		t.Error("writer never closed after the hung write returned")
	}
	if w.closeDuringWrite.Load() {
		t.Error("writer closed while a write was running")
	}
	if w.writeAfterClose.Load() {
		t.Error("writer written after it was closed")
	}
}

func TestGeneralLogger_StopReportsAChildThatTimedOut(t *testing.T) {
	w := newGateWriter()
	t.Cleanup(func() { close(w.release) })
	logger := newGatedLogger(t, w, general_logger.WithStopTimeout(30*time.Millisecond))
	child := logger.AddChild(nil, "child", "", nil)

	child.Info("c1")
	waitEntered(t, w)
	child.Info("c2")
	err := logger.Stop()

	if err == nil {
		t.Error("Stop() error got = nil, want the child's timeout error")
	}
}

func TestGeneralLogger_StopWaitsOneTimeoutForAllChildren(t *testing.T) {
	const timeout = 100 * time.Millisecond
	w := newGateWriter()
	t.Cleanup(func() { close(w.release) })
	logger := newGatedLogger(t, w, general_logger.WithStopTimeout(timeout))
	for _, name := range []string{"a", "b", "c"} {
		logger.AddChild(nil, name, "", nil).Info(name)
	}
	waitEntered(t, w)

	started := time.Now()
	logger.Stop()
	elapsed := time.Since(started)

	if elapsed > 2*timeout {
		t.Errorf("Stop() took %v, want about %v", elapsed, timeout)
	}
}

func TestGeneralLogger_AddChildAfterStopDoesNotWrite(t *testing.T) {
	w := newCloseRecorder(0)
	logger := newRecordedLogger(t, context.Background(), w)
	logger.Stop()

	child := logger.AddChild(nil, "late", "", nil)
	child.Info("after stop")
	time.Sleep(50 * time.Millisecond)

	if got := w.written.Load(); got != 0 {
		t.Errorf("events written after Stop got = %d, want 0", got)
	}
}

func TestGeneralLogger_StartAfterStopReturnsErrStopped(t *testing.T) {
	logger := newRecordedLogger(t, context.Background(), newCloseRecorder(0))
	logger.Stop()

	err := logger.Start(context.Background())

	if !errors.Is(err, general_logger.ErrStopped) {
		t.Errorf("Start() error got = %v, want %v", err, general_logger.ErrStopped)
	}
}

func TestGeneralLogger_StopTimeoutErrorIsErrStopTimeout(t *testing.T) {
	w := newGateWriter()
	t.Cleanup(func() { close(w.release) })
	logger := newGatedLogger(t, w, general_logger.WithStopTimeout(20*time.Millisecond))

	logger.Info("hangs")
	waitEntered(t, w)
	err := logger.Stop()

	if !errors.Is(err, general_logger.ErrStopTimeout) {
		t.Errorf("Stop() error got = %v, want %v", err, general_logger.ErrStopTimeout)
	}
}

func TestGeneralLogger_ZeroStopTimeoutUsesTheDefault(t *testing.T) {
	const total = 100
	w := newCloseRecorder(0)
	logger := newRecordedLogger(t, context.Background(), w, general_logger.WithStopTimeout(0))

	for i := range total {
		logger.Info(strconv.Itoa(i))
	}
	err := logger.Stop()

	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if got := w.atClose.Load(); got != total {
		t.Errorf("events written before Close got = %d, want %d", got, total)
	}
}

func TestGeneralLogger_FlushIncludesChildren(t *testing.T) {
	const total = 20
	w := newCloseRecorder(2 * time.Millisecond)
	logger := newRecordedLogger(t, context.Background(), w)
	t.Cleanup(func() { logger.Stop() })
	child := logger.AddChild(nil, "child", "", nil)

	for i := range total {
		child.Info(strconv.Itoa(i))
	}
	flushWithin(t, logger)

	if got := w.written.Load(); got != total {
		t.Errorf("child events written after Flush got = %d, want %d", got, total)
	}
}

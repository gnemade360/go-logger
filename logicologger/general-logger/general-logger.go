package general_logger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	logicologger "github.com/gnemade360/go-logger/logicologger"
	"github.com/gnemade360/go-logger/logicologger/decorators"
	static_data_decorator "github.com/gnemade360/go-logger/logicologger/decorators/static-data-decorator"
	"github.com/gnemade360/go-logger/logicologger/formatters"
	general_writer "github.com/gnemade360/go-logger/logicologger/general-writer"
	"github.com/gnemade360/go-logger/logicologger/parsers"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type GeneralLogger struct {
	writer logicologger.LogWriter
	lvl    atomic.Int32
	logicologger.ParentLogger
	*decorators.GeneralLogDecorator
	*parsers.GeneralLogParser
	Formatter formatters.LogFormatter
	*sync.RWMutex
	messageQueueCh  chan logicologger.LogEvent
	flushCh         chan chan struct{}
	queueSize       int
	overflow        OverflowPolicy
	dropped         atomic.Uint64
	droppedReported uint64
	stopTimeout     time.Duration
	run             *loggerRun
	stopped         bool
	prefix          string
	children        []logicologger.ParentLogger
	id              string
	OnLog           func(event logicologger.LogEvent)
	indent          int
	PrefixDelimiter string
}

// loggerRun is the lifecycle of one consumer goroutine, from Start to its exit.
type loggerRun struct {
	closing     chan struct{}
	done        chan struct{}
	closingOnce sync.Once
	stopOnce    sync.Once
	stopErr     error
	inflight    sync.WaitGroup
}

func newLoggerRun() *loggerRun {
	return &loggerRun{
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (r *loggerRun) close() {
	r.closingOnce.Do(func() { close(r.closing) })
}

// OverflowPolicy decides what Log does when the queue is full.
type OverflowPolicy int

const (
	// OverflowDrop drops the event and counts it. The next event written is preceded by a WARN
	// event saying how many were dropped.
	OverflowDrop OverflowPolicy = iota
	// OverflowBlock makes the caller wait for room in the queue.
	OverflowBlock
)

const DefaultQueueSize = 1000

var errStillStopping = errors.New("logicologger: logger is still stopping")

func (g *GeneralLogger) AddChild(useThis logicologger.ParentLogger, prefix string, id string, data map[string]interface{}) logicologger.ParentLogger {
	if g == nil {
		return nil
	}
	g.Lock()
	defer func() {
		g.Unlock()
	}()
	pdml := g.PrefixDelimiter
	if len(pdml) == 0 {
		pdml = "-_-"
	}
	childPrefix := g.Prefix() + pdml + prefix
	childId := childPrefix
	child := &GeneralLogger{
		GeneralLogDecorator: g.GeneralLogDecorator,
		GeneralLogParser:    g.GeneralLogParser,
		Formatter:           g.Formatter,
		RWMutex:             &sync.RWMutex{},
		queueSize:           g.queueSize,
		overflow:            g.overflow,
		stopTimeout:         g.stopTimeout,
		prefix:              childPrefix,
		indent:              g.indent + 1,
		id:                  childId,
		OnLog: func(event logicologger.LogEvent) {
			g.write(event)
		},
	}
	child.lvl.Store(g.lvl.Load())
	g.children = append(g.children, child)
	child.Start(context.Background())
	return child
}

func (g *GeneralLogger) Start(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	g.Lock()
	defer g.Unlock()
	hasRun := g.run != nil
	if hasRun && !g.stopped {
		g.stopOnDone(ctx, g.run)
		return nil
	}
	if hasRun && !isClosed(g.run.done) {
		return errStillStopping
	}
	if g.messageQueueCh == nil {
		queueSize := g.queueSize
		if queueSize <= 0 {
			queueSize = DefaultQueueSize
		}
		g.messageQueueCh = make(chan logicologger.LogEvent, queueSize)
	}
	if g.flushCh == nil {
		g.flushCh = make(chan chan struct{})
	}
	run := newLoggerRun()
	g.run = run
	g.stopped = false
	go g.consume(run)
	g.stopOnDone(ctx, run)
	return nil
}

// stopOnDone stops the run when ctx is cancelled, the same way Stop does.
func (g *GeneralLogger) stopOnDone(ctx context.Context, run *loggerRun) {
	cancelled := ctx.Done()
	if cancelled == nil {
		return
	}
	go func() {
		select {
		case <-cancelled:
			g.stopRun(run)
		case <-run.done:
		}
	}()
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (g *GeneralLogger) consume(run *loggerRun) {
	defer close(run.done)
	for {
		select {
		case <-run.closing:
			run.inflight.Wait()
			g.drain(len(g.messageQueueCh))
			return
		case reply := <-g.flushCh:
			g.drain(len(g.messageQueueCh))
			close(reply)
		case logEvent := <-g.messageQueueCh:
			g.reportDropped()
			g.handle(logEvent)
		}
	}
}

func (g *GeneralLogger) drain(n int) {
	for range n {
		g.reportDropped()
		g.handle(<-g.messageQueueCh)
	}
	g.reportDropped()
}

// reportDropped writes one WARN event for the events dropped since the last report. Only the
// consumer goroutine calls it.
func (g *GeneralLogger) reportDropped() {
	total := g.dropped.Load()
	pending := total - g.droppedReported
	if pending == 0 {
		return
	}
	g.droppedReported = total
	g.handle(logicologger.LogEvent{
		Prefix:  g.prefix,
		Message: fmt.Sprintf("dropped %d log events", pending),
		Time:    time.Now(),
		Data:    map[string]interface{}{"context": nil},
		Level:   logicologger.WARN,
		Indent:  g.indent,
	})
}

// seal stops Log from accepting events for this run. Log registers in run.inflight under the read
// lock, so once seal has held the lock no new sender can appear, and closing releases the ones
// still waiting for room.
func (g *GeneralLogger) seal(run *loggerRun) {
	g.Lock()
	g.stopped = true
	g.Unlock()
	run.close()
}

func (g *GeneralLogger) handle(logEvent logicologger.LogEvent) {
	g.Decorate(&logEvent)
	if len(logEvent.Bytes) == 0 {
		if bytes, err := json.Marshal(logEvent); err == nil {
			logEvent.Bytes = append(bytes, '\n')
		}
	}
	g.write(logEvent)
	if g.OnLog != nil {
		g.OnLog(logEvent)
	}
}

func (g *GeneralLogger) write(msg logicologger.LogEvent) {
	if _, err := g.Parse(msg); err == nil {
		var bytes []byte
		var byteConvErr error
		if g.Formatter != nil {
			bytes, byteConvErr = g.Formatter.Format(msg)
		} else {
			bytes = msg.Bytes
		}

		if byteConvErr == nil {
			if writer := g.Writer(); writer != nil {
				if _, writeErr := writer.Write(bytes); writeErr != nil {
					fmt.Fprintf(os.Stderr, "logicologger: write failed: %v\n", writeErr)
				}
			}
		}
	}
}

// Stop stops the children, stops accepting events, writes everything already queued, then closes
// the writer. It waits at most the stop timeout for the queue to drain, and returns an error when
// that runs out.
func (g *GeneralLogger) Stop() error {
	g.RLock()
	run := g.run
	g.RUnlock()
	if run == nil {
		return nil
	}
	return g.stopRun(run)
}

func (g *GeneralLogger) stopRun(run *loggerRun) error {
	run.stopOnce.Do(func() {
		run.stopErr = g.shutdown(run)
	})
	return run.stopErr
}

func (g *GeneralLogger) shutdown(run *loggerRun) error {
	g.RLock()
	children := g.children
	g.RUnlock()
	for _, child := range children {
		child.Stop()
	}

	g.seal(run)
	timer := time.NewTimer(g.stopTimeout)
	defer timer.Stop()
	var timeoutErr error
	select {
	case <-run.done:
	case <-timer.C:
		timeoutErr = fmt.Errorf("logicologger: stop timed out after %s before the queue was written", g.stopTimeout)
	}

	var closeErr error
	if writer := g.Writer(); writer != nil {
		closeErr = writer.Close()
	}
	return errors.Join(timeoutErr, closeErr)
}

// Flush returns once every event logged before the call has been written, or when ctx is done.
func (g *GeneralLogger) Flush(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	g.RLock()
	run := g.run
	flushCh := g.flushCh
	g.RUnlock()
	if run == nil {
		return nil
	}

	reply := make(chan struct{})
	select {
	case flushCh <- reply:
	case <-run.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-reply:
		return nil
	case <-run.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *GeneralLogger) Level() logicologger.LogLevel {
	return logicologger.LogLevel(g.lvl.Load())
}

func (g *GeneralLogger) SetLevel(lvl logicologger.LogLevel) {
	g.lvl.Store(int32(lvl))
}

func (g *GeneralLogger) Writer() logicologger.LogWriter {
	switch g.Level() {

	default:
		return g.writer
	}
}
func (g *GeneralLogger) Log(level logicologger.LogLevel, msg string, i ...interface{}) {

	if level&g.Level() != level {
		return
	}

	arr := make([]interface{}, 0)
	l := len(i)
	var lastFn func() interface{} = nil
	for index, itm := range i {
		if l-1 == index && itm != nil {
			if lf, ok := itm.(func() interface{}); ok && lf != nil {
				lastFn = lf
			} else {
				arr = append(arr, itm)
			}
		} else {
			arr = append(arr, itm)
		}
	}
	var d interface{}
	if lastFn != nil {
		d = lastFn()
	}

	data := map[string]interface{}{
		"context": d,
	}
	hasArgs := len(arr) > 0
	isFormatted := hasArgs && hasFormatVerb(msg)
	if isFormatted {
		msg = fmt.Sprintf(msg, arr...)
	}
	if hasArgs && !isFormatted {
		data["other"] = arr
	}

	le := logicologger.LogEvent{
		Message: msg,
		Time:    time.Now(),
		Data:    data,
		Prefix:  g.prefix,
		Bytes:   nil,
		Level:   level,
		Indent:  g.indent,
	}
	g.RLock()
	run := g.run
	isAccepting := run != nil && !g.stopped
	if !isAccepting {
		g.RUnlock()
		return
	}
	run.inflight.Add(1)
	g.RUnlock()
	defer run.inflight.Done()

	select {
	case g.messageQueueCh <- le:
		return
	default:
	}
	if g.overflow != OverflowBlock {
		g.dropped.Add(1)
		return
	}
	select {
	case g.messageQueueCh <- le:
	case <-run.closing:
	}
}

// Dropped returns how many events were dropped because the queue was full.
func (g *GeneralLogger) Dropped() uint64 {
	return g.dropped.Load()
}

func hasFormatVerb(msg string) bool {
	for i := 0; i < len(msg)-1; i++ {
		if msg[i] != '%' {
			continue
		}
		if msg[i+1] != '%' {
			return true
		}
		i++
	}
	return false
}

func (g *GeneralLogger) Debug(msg string, i ...interface{}) {
	g.Log(logicologger.DEBUG, msg, i...)
}

func (g *GeneralLogger) Info(msg string, i ...interface{}) {
	g.Log(logicologger.INFO, msg, i...)
}

func (g *GeneralLogger) Warn(msg string, i ...interface{}) {
	g.Log(logicologger.WARN, msg, i...)
}

func (g *GeneralLogger) Error(msg string, i ...interface{}) {
	g.Log(logicologger.ERROR, msg, i...)
}

func (g *GeneralLogger) Id() string {
	return g.id
}

func (g *GeneralLogger) Prefix() string {
	return g.prefix
}

func (g *GeneralLogger) SetPrefix(s string) {
	g.prefix = s
}

type GeneralLoggerOption func(*GeneralLogger)

const DefaultLogLevel = logicologger.ERROR | logicologger.WARN | logicologger.INFO

const DefaultStopTimeout = 5 * time.Second

func NewGeneralLogger(ctx context.Context, prefix string, staticData map[string]interface{}, opts ...GeneralLoggerOption) *GeneralLogger {
	obj := &GeneralLogger{
		GeneralLogParser: &parsers.GeneralLogParser{
			Parsers: nil,
		},
		GeneralLogDecorator: &decorators.GeneralLogDecorator{
			Decorators: []decorators.LogDecorators{},
		},
		RWMutex:     &sync.RWMutex{},
		prefix:      prefix,
		id:          prefix,
		queueSize:   DefaultQueueSize,
		stopTimeout: DefaultStopTimeout,
	}
	obj.lvl.Store(int32(DefaultLogLevel))
	if staticData == nil {
		staticData = make(map[string]interface{})
	}
	staticData["prefix"] = prefix
	//if staticData != nil && len(staticData) > 0 {
	obj.Decorators = append(obj.Decorators, &static_data_decorator.StaticDataDecorator{
		Data: staticData,
	})
	//}
	for _, opt := range opts {
		opt(obj)
	}
	obj.Start(ctx)
	return obj
}

// WithWriter writes every event to each of w in turn. Stop closes them, and so does the
// GeneralWriter's finalizer, so wrap a writer that must stay open, such as os.Stdout, in one
// whose Close does nothing.
func WithWriter(w ...io.WriteCloser) GeneralLoggerOption {
	genWriter := general_writer.NewGeneralWriter(w...)
	return func(e *GeneralLogger) {
		e.writer = genWriter
	}
}

// WithStopTimeout sets how long Stop waits for queued events to be written. The default is
// DefaultStopTimeout.
func WithStopTimeout(d time.Duration) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.stopTimeout = d
	}
}

// WithQueueSize sets how many events can wait to be written. The default is DefaultQueueSize,
// which is also used for n <= 0.
func WithQueueSize(n int) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.queueSize = n
	}
}

// WithOverflow sets what Log does when the queue is full. The default is OverflowDrop.
func WithOverflow(p OverflowPolicy) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.overflow = p
	}
}

func WithLogParser(parser ...logicologger.LogParser) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.Parsers = append(logger.Parsers, parser...)
	}
}

func WithLogLevel(level logicologger.LogLevel) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.SetLevel(level)
	}
}

func WithParser(parsers ...logicologger.LogParser) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		if logger.Parsers == nil {
			logger.Parsers = parsers
		} else {
			logger.Parsers = append(logger.Parsers, parsers...)
		}
	}
}

func WithDecorators(decors ...decorators.LogDecorators) GeneralLoggerOption {
	return func(e *GeneralLogger) {
		if e.Decorators == nil {
			e.Decorators = make([]decorators.LogDecorators, 0)
		}
		e.Decorators = append(e.Decorators, decors...)
	}
}

func WithFormatter(f formatters.LogFormatter) GeneralLoggerOption {
	return func(logger *GeneralLogger) {
		logger.Formatter = f
	}
}

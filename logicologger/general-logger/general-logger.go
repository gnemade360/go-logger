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
		messageQueueCh:      make(chan logicologger.LogEvent, 1000),
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
		return nil
	}
	if hasRun && !isClosed(g.run.done) {
		return errStillStopping
	}
	if g.messageQueueCh == nil {
		g.messageQueueCh = make(chan logicologger.LogEvent, 1000)
	}
	run := newLoggerRun()
	g.run = run
	g.stopped = false
	go g.consume(ctx, run)
	return nil
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (g *GeneralLogger) consume(ctx context.Context, run *loggerRun) {
	defer close(run.done)
	for {
		select {
		case <-run.closing:
			return
		case <-ctx.Done():
			g.seal(run)
			return
		case logEvent := <-g.messageQueueCh:
			g.handle(logEvent)
		}
	}
}

// seal stops Log from accepting events for this run.
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

func (g *GeneralLogger) Stop() error {
	g.Lock()
	run := g.run
	g.stopped = true
	children := g.children
	g.Unlock()

	if run != nil {
		run.close()
	}
	if writer := g.Writer(); writer != nil {
		writer.Close()
	}
	for _, child := range children {
		child.Stop()
	}
	return nil
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
	defer g.RUnlock()
	isAccepting := g.run != nil && !g.stopped
	if !isAccepting {
		return
	}
	select {
	case g.messageQueueCh <- le:
	default:
	}
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

func NewGeneralLogger(ctx context.Context, prefix string, staticData map[string]interface{}, opts ...GeneralLoggerOption) *GeneralLogger {
	obj := &GeneralLogger{
		GeneralLogParser: &parsers.GeneralLogParser{
			Parsers: nil,
		},
		GeneralLogDecorator: &decorators.GeneralLogDecorator{
			Decorators: []decorators.LogDecorators{},
		},
		RWMutex:        &sync.RWMutex{},
		messageQueueCh: make(chan logicologger.LogEvent, 1000),
		prefix:         prefix,
		id:             prefix,
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

func WithWriter(w ...io.WriteCloser) GeneralLoggerOption {
	genWriter := general_writer.NewGeneralWriter(w...)
	return func(e *GeneralLogger) {
		e.writer = genWriter
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

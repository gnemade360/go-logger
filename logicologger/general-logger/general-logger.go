package general_logger

import (
	"context"
	"encoding/json"
	"fmt"
	logicologger "github.com/gnemade360/go-logger/logicologger"
	"github.com/gnemade360/go-logger/logicologger/decorators"
	static_data_decorator "github.com/gnemade360/go-logger/logicologger/decorators/static-data-decorator"
	"github.com/gnemade360/go-logger/logicologger/formatters"
	general_writer "github.com/gnemade360/go-logger/logicologger/general-writer"
	"github.com/gnemade360/go-logger/logicologger/parsers"
	"io"
	"sync"
	"time"
)

var (
	singletonLogger logicologger.ParentLogger
	once            sync.Once
)

type GeneralLogger struct {
	writer logicologger.LogWriter
	lvl    logicologger.LogLevel
	logicologger.ParentLogger
	*decorators.GeneralLogDecorator
	*parsers.GeneralLogParser
	Formatter formatters.LogFormatter
	*sync.RWMutex
	messageQueueCh  chan logicologger.LogEvent
	stopch          chan bool
	started         bool
	stopped         bool
	prefix          string
	children        []logicologger.ParentLogger
	id              string
	OnLog           func(event logicologger.LogEvent)
	indent          int
	PrefixDelimiter string
	once            sync.Once
}

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
	useThis = &GeneralLogger{
		//writer:              g.writer,
		lvl:                 g.lvl,
		GeneralLogDecorator: g.GeneralLogDecorator,
		GeneralLogParser:    g.GeneralLogParser,
		Formatter:           g.Formatter,
		RWMutex:             &sync.RWMutex{},
		messageQueueCh:      make(chan logicologger.LogEvent, 1000),
		started:             false,
		stopped:             false,
		prefix:              childPrefix,
		indent:              g.indent + 1,
		id:                  childId,
		OnLog: func(event logicologger.LogEvent) {
			fmt.Printf("\nIn Child Log-event: %v\n", event)
			g.write(event)
		},
		stopch: make(chan bool),
	}
	g.children = append(g.children, useThis)
	useThis.Start(context.Background())
	return useThis
}

func (g *GeneralLogger) Start(ctx context.Context) error {
	if g == nil {
		return nil
	}
	var cancelFunc context.Context = context.Background()
	if ctx != nil {
		cancelFunc = ctx
	}
	g.RWMutex.Lock()
	//defer func() {
	//    g.RWMutex.Unlock()
	//}()
	g.started = true
	g.stopped = false
	if g.messageQueueCh == nil {
		g.messageQueueCh = make(chan logicologger.LogEvent, 1000)
	}
	g.RWMutex.Unlock()

	go func() {
		for {
			select {
			case <-g.stopch:
			case <-cancelFunc.Done():
				close(g.messageQueueCh)
				g.messageQueueCh = nil // added for datarace
			case logEvent, ok := <-g.messageQueueCh:
				if !ok {
					g.Lock()
					g.messageQueueCh = nil // added for datarace
					g.Unlock()
				} else {
					// result of the channel
					g.Decorate(&logEvent)
					if logEvent.Bytes == nil || len(logEvent.Bytes) == 0 {
						var er error
						bytes, er := json.Marshal(logEvent)
						if er == nil {
							logEvent.Bytes = bytes
						}
					}

					g.write(logEvent)
					if g.OnLog != nil {
						g.OnLog(logEvent)
					}
				}
			}
			if g.messageQueueCh == nil {
				break
			}
		}
	}()

	return nil
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
			go func() {
				if writer := g.Writer(); writer != nil {
					writer.Write(bytes)
				} else {
					fmt.Printf("\nnil-w: %v\n", writer)
				}

			}()
		}
	}
}

func (g *GeneralLogger) Stop() error {
	g.Lock()

	g.started = false
	g.stopped = true
	if writer := g.Writer(); writer != nil {
		writer.Close()
	}
	// closing children
	if g.children != nil && len(g.children) > 0 {
		for _, child := range g.children {
			child.Stop()
		}
	}

	//if g.messageQueueCh != nil && !g.isMessageQueueClosed() {

	//close(g.messageQueueCh)
	//g.messageQueueCh = nil // added for datarace
	g.stopch <- true
	//}
	g.Unlock()
	return nil
}
func (g *GeneralLogger) isMessageQueueClosed() bool {
	select {
	case <-g.messageQueueCh:
		return true
	default:
	}

	return false
}

func (g *GeneralLogger) Level() logicologger.LogLevel {
	return g.lvl
}

func (g *GeneralLogger) SetLevel(lvl logicologger.LogLevel) {
	g.lvl = lvl
}

func (g *GeneralLogger) Writer() logicologger.LogWriter {
	switch g.Level() {

	default:
		return g.writer
	}
}
func (g *GeneralLogger) Log(level logicologger.LogLevel, msg string, i ...interface{}) {

	if level&(g.lvl) != level {
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

	le := logicologger.LogEvent{
		Message: msg,
		Time:    time.Now(),
		Data: map[string]interface{}{
			"context": d,
			"other":   arr,
		},
		Prefix: g.prefix,
		Bytes:  nil,
		Level:  level,
		Indent: g.indent,
	}
	if g.prefix != "root:" {
		fmt.Printf("\nprefix-g.prefix: %v\n", g.prefix)
	}
	go func() {
		if g.prefix != "root:" {
			fmt.Printf("\nprefix-g.prefix: %v\n", g.prefix)
		}
		g.messageQueueCh <- le
	}()
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

func (g *GeneralLogger) Prefix() string {
	return g.prefix
}

func (g *GeneralLogger) SetPrefix(s string) {
	g.prefix = s
}

type GeneralLoggerOption func(*GeneralLogger)

func NewGeneralLogger(ctx context.Context, prefix string, staticData map[string]interface{}, opts ...GeneralLoggerOption) *GeneralLogger {
	obj := &GeneralLogger{
		GeneralLogParser: &parsers.GeneralLogParser{
			Parsers: nil,
		},
		GeneralLogDecorator: &decorators.GeneralLogDecorator{
			Decorators: []decorators.LogDecorators{},
		},
		RWMutex:        &sync.RWMutex{},
		messageQueueCh: make(chan logicologger.LogEvent),
		prefix:         prefix,
		id:             prefix,
		stopch:         make(chan bool),
	}
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
		logger.lvl = level
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

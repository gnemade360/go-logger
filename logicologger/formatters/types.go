package formatters

import logicologger "github.com/gnemade360/go-logger/logicologger"

type LogFormatter interface {
	Format(event logicologger.LogEvent) ([]byte, error)
}

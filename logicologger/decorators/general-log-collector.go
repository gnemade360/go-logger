package decorators

import (
	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type LogDecorators interface {
	Decorate(*logicologger.LogEvent) error
}

type GeneralLogDecorator struct {
	Decorators []LogDecorators
}

func (c *GeneralLogDecorator) Decorate(e *logicologger.LogEvent) error {
	if c.Decorators == nil || len(c.Decorators) == 0 {
		return nil
	}

	for _, decorator := range c.Decorators {
		cc := decorator
		cc.Decorate(e)
	}
	return nil
}

type GeneralLogCollectorOption func(*GeneralLogDecorator)

func NewGeneralLogCollector(opts ...GeneralLogCollectorOption) *GeneralLogDecorator {
	obj := &GeneralLogDecorator{}
	for _, opt := range opts {
		opt(obj)
	}
	return obj
}

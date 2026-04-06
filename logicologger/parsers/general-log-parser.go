package parsers

import logicologger "github.com/gnemade360/go-logger/logicologger"

type GeneralLogParser struct {
	Parsers []logicologger.LogParser
}

func (gp *GeneralLogParser) Parse(e logicologger.LogEvent) (logicologger.LogEvent, error) {
	if gp == nil {
		return logicologger.LogEvent{}, nil
	}
	if gp.Parsers != nil && len(gp.Parsers) > 0 {
		for _, parser := range gp.Parsers {
			parser.Parse(e)
		}
	}
	return e, nil
}

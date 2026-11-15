package json_line_formatter

import (
	"encoding/json"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

const (
	keyTime    = "ts"
	keyLevel   = "level"
	keyLogger  = "logger"
	keyMessage = "msg"
	keyContext = "context"
	keyOther   = "other"
)

// JSONLineFormatter writes one JSON object per line with stable keys: ts, level, logger and msg,
// then the static data and the trailing-function context at the top level. Keys are sorted.
// Reserved keys win over data keys, context keys win over static data keys, and positional
// args ("other") are never written. A context that is not a map is written under "context".
type JSONLineFormatter struct {
	TimeFormat string // default time.RFC3339Nano, always in UTC
}

func (f JSONLineFormatter) Format(event logicologger.LogEvent) ([]byte, error) {
	timeFormat := f.TimeFormat
	if timeFormat == "" {
		timeFormat = time.RFC3339Nano
	}

	line := make(map[string]interface{}, len(event.Data)+4)
	for k, v := range event.Data {
		isSkipped := k == keyContext || k == keyOther
		if isSkipped {
			continue
		}
		line[k] = v
	}
	addContext(line, event.Data[keyContext])
	line[keyTime] = event.Time.UTC().Format(timeFormat)
	line[keyLevel] = logicologger.LevelName(event.Level)
	line[keyLogger] = event.Prefix
	line[keyMessage] = event.Message

	out, err := json.Marshal(line)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func addContext(line map[string]interface{}, context interface{}) {
	switch fields := context.(type) {
	case nil:
	case map[string]interface{}:
		for k, v := range fields {
			line[k] = v
		}
	case logicologger.LogData:
		for k, v := range fields {
			line[k] = v
		}
	default:
		line[keyContext] = fields
	}
}

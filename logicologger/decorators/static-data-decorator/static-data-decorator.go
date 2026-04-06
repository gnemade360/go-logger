package static_data_decorator

import (
	"encoding/json"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type StaticDataDecorator struct {
	Data map[string]interface{}
}

func (c *StaticDataDecorator) Decorate(entry *logicologger.LogEvent) error {
	if c == nil || c.Data == nil || len(c.Data) == 0 {
		return nil
	}
	newMap := map[string]interface{}{}
	if byts, er := json.Marshal(c.Data); er == nil {
		if e := json.Unmarshal(byts, &newMap); e != nil {
			return nil
		}
	}

	if entry.Data == nil {

		entry.Data = newMap

	} else {
		for k, v := range newMap {
			entry.Data[k] = v
		}
	}
	return nil
}

package template_formatter_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
	caller_collector "github.com/gnemade360/go-logger/logicologger/decorators/caller-collector"
	template_formatter "github.com/gnemade360/go-logger/logicologger/formatters/template-formatter"
)

func formatToEvent(t *testing.T, f template_formatter.TemplateFormatter, event logicologger.LogEvent) logicologger.LogEvent {
	t.Helper()
	gotB, err := f.Format(event)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	var got logicologger.LogEvent
	if err := json.Unmarshal(gotB, &got); err != nil {
		t.Fatalf("Format() output is not a JSON event: %v, output %q", err, gotB)
	}
	return got
}

func TestTemplateFormatter_DefaultTemplateKeepsMessage(t *testing.T) {
	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{name: "scalar args", data: map[string]interface{}{"other": []interface{}{"first", "second"}}},
		{name: "map arg", data: map[string]interface{}{"other": []interface{}{map[string]interface{}{"name": "parrot"}}}},
		{name: "nil callers", data: map[string]interface{}{"callers": nil}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := template_formatter.TemplateFormatter{RWMutex: &sync.RWMutex{}}
			event := logicologger.LogEvent{Message: "Starting task", Time: time.Now(), Data: tt.data}

			got := formatToEvent(t, f, event)

			if got.Message != "Starting task" {
				t.Errorf("Message got = %q, want %q", got.Message, "Starting task")
			}
		})
	}
}

func TestTemplateFormatter_IncludeCallersRendersFirstCaller(t *testing.T) {
	f := template_formatter.TemplateFormatter{RWMutex: &sync.RWMutex{}, IncludeCallers: true}
	event := logicologger.LogEvent{
		Message: "Starting task",
		Time:    time.Now(),
		Data: map[string]interface{}{
			"callers": []*caller_collector.Caller{{Struct: "Worker", Func: "Run"}},
		},
	}

	got := formatToEvent(t, f, event)

	if !strings.Contains(got.Message, "Worker.Run") {
		t.Errorf("Message got = %q, want it to contain %q", got.Message, "Worker.Run")
	}
}

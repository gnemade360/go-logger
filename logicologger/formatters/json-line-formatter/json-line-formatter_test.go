package json_line_formatter_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
	json_line_formatter "github.com/gnemade360/go-logger/logicologger/formatters/json-line-formatter"
)

func formatLine(t *testing.T, f json_line_formatter.JSONLineFormatter, event logicologger.LogEvent) map[string]interface{} {
	t.Helper()
	out, err := f.Format(event)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if !bytes.HasSuffix(out, []byte("\n")) || bytes.Count(out, []byte("\n")) != 1 {
		t.Fatalf("Format() output is not a single line: %q", out)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("Format() output is not JSON: %v, output %q", err, out)
	}
	return got
}

func TestJSONLineFormatter_Format(t *testing.T) {
	berlin := time.FixedZone("CET", 3600)
	event := logicologger.LogEvent{
		Prefix:  "gateway",
		Message: "request done",
		Time:    time.Date(2026, 11, 15, 10, 0, 0, 5, berlin),
		Level:   logicologger.WARN,
		Data: logicologger.LogData{
			"service": "llm",
			"status":  "static value loses to the context",
			"msg":     "static value loses to the message",
			"other":   []interface{}{"secret"},
			"context": map[string]interface{}{"status": float64(200), "level": "context value loses to the level"},
		},
	}

	got := formatLine(t, json_line_formatter.JSONLineFormatter{}, event)

	want := map[string]interface{}{
		"ts":      "2026-11-15T09:00:00.000000005Z",
		"level":   "WARN",
		"logger":  "gateway",
		"msg":     "request done",
		"service": "llm",
		"status":  float64(200),
	}
	if len(got) != len(want) {
		t.Errorf("keys got = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s got = %v, want %v", k, got[k], v)
		}
	}
}

func TestJSONLineFormatter_LogDataContextIsFlattened(t *testing.T) {
	event := logicologger.LogEvent{
		Data: logicologger.LogData{"context": logicologger.LogData{"request_id": "r1"}},
	}

	got := formatLine(t, json_line_formatter.JSONLineFormatter{}, event)

	if got["request_id"] != "r1" {
		t.Errorf("request_id got = %v, want %q", got["request_id"], "r1")
	}
	if _, hasContext := got["context"]; hasContext {
		t.Errorf("context key got = %v, want it flattened", got["context"])
	}
}

func TestJSONLineFormatter_LevelNames(t *testing.T) {
	tests := []struct {
		level logicologger.LogLevel
		want  string
	}{
		{level: logicologger.ERROR, want: "ERROR"},
		{level: logicologger.WARN, want: "WARN"},
		{level: logicologger.INFO, want: "INFO"},
		{level: logicologger.DEBUG, want: "DEBUG"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatLine(t, json_line_formatter.JSONLineFormatter{}, logicologger.LogEvent{Level: tt.level})

			if got["level"] != tt.want {
				t.Errorf("level got = %v, want %q", got["level"], tt.want)
			}
		})
	}
}

func TestJSONLineFormatter_CustomTimeFormatAndNonMapContext(t *testing.T) {
	event := logicologger.LogEvent{
		Time: time.Date(2026, 11, 15, 10, 0, 0, 0, time.FixedZone("X", -7200)),
		Data: logicologger.LogData{"context": "plain"},
	}

	got := formatLine(t, json_line_formatter.JSONLineFormatter{TimeFormat: time.RFC3339}, event)

	if got["ts"] != "2026-11-15T12:00:00Z" {
		t.Errorf("ts got = %v, want %q", got["ts"], "2026-11-15T12:00:00Z")
	}
	if got["context"] != "plain" {
		t.Errorf("context got = %v, want %q", got["context"], "plain")
	}
}

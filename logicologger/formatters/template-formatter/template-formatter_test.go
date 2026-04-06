package template_formatter_test

import (
	"reflect"
	"sync"
	"testing"
	"text/template"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
	template_formatter "github.com/gnemade360/go-logger/logicologger/formatters/template-formatter"
)

func TestTemplateFormatter_Format(t1 *testing.T) {
	type fields struct {
		Str      string
		template template.Template
	}
	type args struct {
		event logicologger.LogEvent
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		// TODO: Add test cases.
		{
			name: "1",
			fields: fields{
				Str: "",
			},
			args: args{
				event: logicologger.LogEvent{
					Message: "Starting task",
					Time:    time.Now(),
					Data: map[string]interface{}{
						"other": []interface{}{"first", "second"},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "2",
			fields: fields{
				Str: "",
			},
			args: args{
				event: logicologger.LogEvent{
					Message: "Starting task",
					Time:    time.Now(),
					Data: map[string]interface{}{
						"other": []interface{}{map[string]interface{}{
							"name": "parrot",
						}},
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "2",
			fields: fields{
				Str: "",
			},
			args: args{
				event: logicologger.LogEvent{
					Message: "Starting task", Time: time.Now(), Data: map[string]interface{}{
						"callers": nil,
					},
				},
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := template_formatter.TemplateFormatter{
				MessageTemplate: tt.fields.Str,
				RWMutex:         &sync.RWMutex{},
			}
			gotB, err := t.Format(tt.args.event)
			got := string(gotB)
			if (err != nil) != tt.wantErr {
				t1.Errorf("Format() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("Format() got = %v, want %v", got, tt.want)
			}
		})
	}
}

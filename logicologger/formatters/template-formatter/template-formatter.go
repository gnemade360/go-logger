package template_formatter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"text/template"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type TemplateFormatter struct {
	MessageTemplate string
	template        *template.Template
	RWMutex         *sync.RWMutex
	IncludeCallers  bool
}

func (t *TemplateFormatter) getTemplate() *template.Template {
	t.RWMutex.RLock()

	if t.template != nil {
		t.RWMutex.RUnlock()
		return t.template
	}
	t.RWMutex.RUnlock()
	t.RWMutex.Lock()
	defer func() {
		t.RWMutex.Unlock()
	}()
	if len(t.MessageTemplate) == 0 {
		if t.IncludeCallers {
			t.MessageTemplate = `
           {{.Time}} -- {{.Message}}
           {{- if  .Data.callers  }}
               {{(index .Data.callers 0).MessageTemplateuct}}.{{(index .Data.callers 0).Func}}============>
           {{- end}}
           {{- if IsNotNil .Data.other}}
               {{- range $key, $val := .Data.other}}
                   {{- if or (IsMap $val) (IsSlice $val) }}
                       {{- range $k, $v :=  $val}}
                           {{$k}} --> {{$v}}
                       {{- end}}
                   {{- else}}
                           {{ $val}}
                   {{- end}}
               {{- end}}
           {{- end}}
           `

		} else {
			t.MessageTemplate = "{{.Message}}"
		}
	}
	funcMap := template.FuncMap{
		"IsMap": func(map1 interface{}) bool {
			return map1 != nil && reflect.ValueOf(map1).Kind() == reflect.Map
		},
		"IsSlice": func(slice interface{}) bool {
			return slice != nil && reflect.TypeOf(slice).Kind() == reflect.Slice
		},
		"IsNil": func(obj interface{}) bool {
			return obj == nil
		},
		"IsNotNil": func(obj interface{}) bool {
			return obj != nil
		},
	}

	t.template = template.New("loggerTemplate").Funcs(funcMap)
	return t.template
}

func (t TemplateFormatter) Format(event logicologger.LogEvent) ([]byte, error) {
	tmpl := t.getTemplate()
	t.RWMutex.Lock()
	defer t.RWMutex.Unlock()
	_, err := tmpl.Parse(t.MessageTemplate)

	if err != nil {
		panic(err)
	}

	var doc bytes.Buffer
	if errr := tmpl.Execute(&doc, event); errr != nil {
		fmt.Printf("\nerr while tmpl-err: %v\n", errr)
	}

	tmplMsg := template.New("loggerTemplateMessage")
	if _, errWhileMsgTmpl := tmplMsg.Parse(doc.String()); errWhileMsgTmpl != nil {
		fmt.Printf("\nerrWhileMsgTmpl while templating in logging-errWhileMsgTmpl: %v\n", errWhileMsgTmpl)
	}

	var docMsg bytes.Buffer
	err = tmplMsg.Execute(&docMsg, event)
	if err != nil {
		panic(err)
	}
	msg := docMsg.String()
	byts := event.Bytes
	if event.Message != msg || len(byts) == 0 {
		event.Message = msg
		event.Bytes = []byte{}
		if b, e := json.Marshal(event); e == nil {
			byts = b
		} else {
			fmt.Printf("\nJSON marshal error-e: %v\n", e)
		}

	}

	return byts, nil
}

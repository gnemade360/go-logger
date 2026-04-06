package message_only_formatter

import (
	"bytes"
	"fmt"
	"reflect"
	"sync"
	"text/template"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type MessageOnlyFormatter struct {
	Str      string
	template *template.Template
	RWMutex  *sync.RWMutex
}

func (t *MessageOnlyFormatter) getTemplate() *template.Template {
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
	if len(t.Str) == 0 {
		t.Str = `
            {{.Time}} -- {{.Message}}
            {{- if  .Data.callers  }}
                {{(index .Data.callers 0).Struct}}.{{(index .Data.callers 0).Func}}============>
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

func (t MessageOnlyFormatter) Format(event logicologger.LogEvent) ([]byte, error) {
	tmpl := t.getTemplate()
	t.RWMutex.Lock()
	defer t.RWMutex.Unlock()
	_, err := tmpl.Parse(t.Str)

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

	return docMsg.Bytes(), nil
}

package http_log_writer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	logicologger "github.com/gnemade360/go-logger/logicologger"
)

type DataPoint struct {
	Measurement string                 `json:"measurement"`
	Tags        map[string]string      `json:"tags"`
	Fields      map[string]interface{} `json:"fields"`
	Time        time.Time              `json:"time"`
}

type HTTPLogWriter struct {
	EndPoint     string
	Method       string
	Host         string
	Port         int
	HttpProtocol string
	Client       *http.Client
}

func (f *HTTPLogWriter) Write(p []byte) (n int, err error) {
	event := logicologger.LogEvent{}
	er := json.Unmarshal(p, &event)
	if er != nil {
		return 0, er
	}
	data := f.Parse(event)
	dpBytes, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%v://%v:%v/%v", f.HttpProtocol, f.Host, f.Port, f.EndPoint)

	req, err := http.NewRequest(f.Method, url, bytes.NewBuffer(dpBytes))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	req.Header.Set("Authorization", "Token 5up3r-S3cr3t-auth-t0k3n")

	resp, err := f.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected response status code: %v", resp.StatusCode)
	}

	return len(p), nil
}

func (f *HTTPLogWriter) Close() error {
	if tr, ok := f.Client.Transport.(*http.Transport); ok {
		tr.CloseIdleConnections()
	}
	return nil
}

func (f *HTTPLogWriter) Parse(event logicologger.LogEvent) logicologger.LogData {
	data := event.Data
	if other, ok := data["other"]; ok {
		if otherItems, isarr := other.([]interface{}); isarr && otherItems != nil {
			for _, obj := range otherItems {
				if objMp, isMap := obj.(map[string]interface{}); objMp != nil && isMap {
					for k, v := range objMp {
						data[k] = v
					}
				}

			}
		}
	}
	toPost := &DataPoint{
		Measurement: "debug-measurement",
		Tags:        map[string]string{},
		Fields:      map[string]interface{}{},
		Time:        event.Time,
	}
	for k, v := range data {
		if strings.HasPrefix(k, "tag-") {
			nk := strings.Replace(k, "tag-", "", 1)
			toPost.Tags[nk] = v.(string)
		} else {
			toPost.Fields[k] = v
		}
	}
	return data
}

func NewHTTPLogWriter(protocol, host string, port int, endpoint, method string) *HTTPLogWriter {
	return &HTTPLogWriter{
		EndPoint:     endpoint,
		Method:       method,
		Host:         host,
		Port:         port,
		HttpProtocol: protocol,
		Client:       &http.Client{},
	}
}

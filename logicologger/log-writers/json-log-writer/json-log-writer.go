package json_log_writer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type JsonLogWriter struct {
	filename string
	file     *os.File
	logs     map[string][][]byte
	mu       sync.Mutex
}

func (w *JsonLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer func() {
		w.mu.Unlock()
	}()
	if w.logs == nil {
		return 0, fmt.Errorf("logs map not setup with JsonLogWriter")
	}
	w.logs["logs"] = append(w.logs["logs"], p)
	return len(p), nil
}

func (w *JsonLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	start := time.Now()

	if w.file != nil {
		if w.logs == nil {
			return fmt.Errorf("logs map not setup with JsonLogWriter")
		}

		if logs, ok := w.logs["logs"]; ok {
			lsCh := make(chan map[string]interface{}, len(logs))
			g, _ := errgroup.WithContext(context.Background())
			for _, lg := range logs {
				lbytes := lg

				g.Go(func() error {
					mp := map[string]interface{}{}
					if er := json.Unmarshal(lbytes, &mp); er == nil {
						str := string(lbytes)
						fmt.Printf("\nlbytes-lg: %v\n", str)
						if strings.Contains(str, "Sample") {
							fmt.Printf("\nlg-str: %v\n", str)
						}
						lsCh <- mp
					} else {
						msg := string(lbytes)
						newMp := map[string]interface{}{
							"Message": msg,
						}
						lsCh <- newMp
					}
					return nil
				})
			}
			if err := g.Wait(); err != nil {
				fmt.Printf("\nError:%v\n", err)
			}
			close(lsCh)
			lgs := make([]map[string]interface{}, 0)
			for ch := range lsCh {
				lgs = append(lgs, ch)
			}
			lgsNew := map[string][]map[string]interface{}{
				"logs": lgs,
			}
			jsonBytes, err := json.Marshal(lgsNew)
			if err == nil {
				str := string(jsonBytes)
				os.WriteFile(w.filename, []byte(str), 0666)
			}
		}

	}
	elapsed := time.Since(start)

	fmt.Printf("Elapsed time for writing logs: %v\n", elapsed)
	return nil
}

func NewJsonLogWriter(filename string) (*JsonLogWriter, error) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return &JsonLogWriter{
		filename: filename,
		file:     file,
		logs:     map[string][][]byte{"logs": {}},
	}, nil
}

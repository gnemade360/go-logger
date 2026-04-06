package map_log_writer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type MapLogWriter struct {
	logs map[string]interface{}
	mu   sync.Mutex
}

func (w *MapLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer func() {
		w.mu.Unlock()
	}()
	if w.logs == nil {
		return 0, fmt.Errorf("logs map not setup with MapLogWriter")
	}
	var m map[string]interface{}
	if err := json.Unmarshal(p, &m); err != nil {
		return 0, err
	}
	if _, ok := w.logs["logs"]; !ok {
		w.logs["logs"] = []map[string]interface{}{}
	}
	w.logs["logs"] = append(w.logs["logs"].([]map[string]interface{}), m)
	return len(p), nil
}

func (w *MapLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	start := time.Now()

	if w.logs == nil {
		return fmt.Errorf("logs map not setup with MapLogWriter")
	}

	elapsed := time.Since(start)
	fmt.Printf("Elapsed time for writing logs: %v\n", elapsed)
	return nil
}

func NewMapLogWriter() *MapLogWriter {
	return &MapLogWriter{
		logs: map[string]interface{}{
			"logs": []map[string]interface{}{},
		},
	}
}

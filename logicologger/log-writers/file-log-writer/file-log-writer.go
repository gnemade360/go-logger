package file_log_writer

import (
	"fmt"
	"os"
	"sync"
)

type FileLogWriter struct {
	filename string
	file     *os.File
	mu       sync.Mutex
}

func (w *FileLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer func() {
		w.mu.Unlock()
	}()
	if w.file == nil {
		return 0, fmt.Errorf("file handler not setup with FileLogWriter")
	}
	return w.file.Write(p)
}

func (w *FileLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		if err != nil {
			return fmt.Errorf("failed to close file: %v", err)
		}
		w.file = nil
	}

	return nil
}

func NewFileLogWriter(filename string) (*FileLogWriter, error) {
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}

	return &FileLogWriter{
		filename: filename,
		file:     file,
	}, nil
}

package general_writer

import (
	"io"
	"runtime"
	"sync"

	multi_error "github.com/gnemade360/go-logger/logicologger/multi-error"
)

type GeneralWriter struct {
	writers []io.WriteCloser
}

func NewGeneralWriter(writers ...io.WriteCloser) *GeneralWriter {
	g := &GeneralWriter{
		writers: writers,
	}
	runtime.SetFinalizer(g, func(g *GeneralWriter) {
		g.Close()
	})
	return g
}

func (g *GeneralWriter) Write(p []byte) (n int, err error) {
	var mu sync.RWMutex
	multiErr := &multi_error.MultiError{
		RWMutex: &mu,
	}
	wg := &sync.WaitGroup{}
	for _, w := range g.writers {
		wg.Add(1)
		go func(w io.WriteCloser) {

			w.Write(p)
			wg.Done()
		}(w)
	}

	wg.Wait()
	return len(p), multiErr.ErrorOrNil()
}

func (g *GeneralWriter) Close() error {
	var mu sync.RWMutex
	multiErr := &multi_error.MultiError{
		RWMutex: &mu,
	}

	var wg sync.WaitGroup
	for _, w := range g.writers {
		wg.Add(1)
		go func(w io.WriteCloser) {
			defer wg.Done()

			closeErr := w.Close()
			if closeErr != nil {
				multiErr.Add(closeErr)
			}
		}(w)
	}

	wg.Wait()

	return multiErr.ErrorOrNil()
}

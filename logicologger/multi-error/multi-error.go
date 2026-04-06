package multi_error

import (
	"bytes"
	"sync"
)

type MultiError struct {
	errors []error
	*sync.RWMutex
}

func (m *MultiError) Add(err error) {
	if err != nil {
		m.Lock()
		defer func() {
			m.Unlock()
		}()
		m.errors = append(m.errors, err)
	}
}

func (m *MultiError) Error() string {
	m.RLock()
	defer m.RUnlock()

	if len(m.errors) == 0 {
		return ""
	}

	var buf bytes.Buffer
	for i, err := range m.errors {
		if err != nil {
			if i > 0 {
				buf.WriteString("; ")
			}
			buf.WriteString(err.Error())
		}
	}
	return buf.String()
}

func (m *MultiError) ErrorOrNil() error {
	m.RLock()
	defer func() {
		m.RUnlock()
	}()

	if len(m.errors) == 0 {
		return nil
	}
	return m
}

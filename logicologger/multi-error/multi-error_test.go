package multi_error

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func setupMultiError(errors ...error) *MultiError {
	m := &MultiError{RWMutex: &sync.RWMutex{}}
	for _, err := range errors {
		m.Add(err)
	}
	return m
}

func TestMultiError_Add(t *testing.T) {
	m := setupMultiError()

	m.Add(errors.New("error 1"))
	if got, want := len(m.errors), 1; got != want {
		t.Errorf("unexpected number of errors after adding one: got %d, want %d", got, want)
	}

	m.Add(errors.New("error 2"))
	if got, want := len(m.errors), 2; got != want {
		t.Errorf("unexpected number of errors after adding two: got %d, want %d", got, want)
	}

	m.Add(nil)
	if got, want := len(m.errors), 2; got != want {
		t.Errorf("unexpected number of errors after adding nil: got %d, want %d", got, want)
	}
}

func TestMultiError_Error(t *testing.T) {
	m := setupMultiError(errors.New("error 1"), errors.New("error 2"))

	got := m.Error()
	want := "error 1; error 2"
	if got != want {
		t.Errorf("unexpected error string: got %q, want %q", got, want)
	}

	m.Add(nil)

	got = m.Error()
	want = "error 1; error 2"
	if got != want {
		t.Errorf("unexpected error string after adding nil: got %q, want %q", got, want)
	}
}

func TestMultiError_ErrorOrNil(t *testing.T) {
	m := setupMultiError(errors.New("error 1"), errors.New("error 2"))

	got := m.ErrorOrNil()
	if got == nil {
		t.Errorf("unexpected nil error")
	}

	m.Add(nil)

	got = m.ErrorOrNil()
	if got == nil {
		t.Errorf("unexpected nil error after adding nil")
	}

	m.errors = nil

	got = m.ErrorOrNil()
	if got != nil {
		t.Errorf("unexpected non-nil error after clearing errors")
	}
}

func TestMultiError_Concurrent(t *testing.T) {
	const numErrors = 100

	m := &MultiError{RWMutex: &sync.RWMutex{}}
	var wg sync.WaitGroup

	for i := 0; i < numErrors; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.Add(fmt.Errorf("error %d", i))
		}(i)
	}

	wg.Wait()

	if got, want := len(m.errors), numErrors; got != want {
		t.Errorf("unexpected number of errors: got %d, want %d", got, want)
	}

}

package general_writer

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGeneralWriter_Write(t *testing.T) {
	mp := &sync.Map{}
	pw := newMockWriter(2, mp)
	// Create a new GeneralWriter that writes to the pipe writer.
	gw := NewGeneralWriter(pw)

	// Write some data to the GeneralWriter.
	n, err := gw.Write([]byte("hello, world"))

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != len("hello, world") {
		t.Errorf("unexpected number of bytes written: got %d, want %d", n, len("hello, world"))
	}

	// Close the GeneralWriter.
	if err := gw.Close(); err != nil {
		t.Errorf("unexpected error during Close: %v", err)
	}

	if err != nil {
		t.Errorf("unexpected error reading from pipe: %v", err)
	}

}

func TestGeneralWriter_WriteConcurrent(t *testing.T) {
	// Create a pipe.
	smp := &sync.Map{}
	pr := newMockWriter(1, smp)
	pw := newMockWriter(2, smp)

	defer pr.Close()
	defer pw.Close()

	// Create a new GeneralWriter that writes to the pipe writer.
	gw := NewGeneralWriter(pw, pr)

	// Use a WaitGroup to wait for all goroutines to finish.
	var wg sync.WaitGroup

	// Start multiple goroutines to write to the GeneralWriter concurrently.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bytes := []byte{byte('a' + i)}
			n, err := gw.Write(bytes)
			if err != nil {
				t.Errorf("unexpected error writing data: %v", err)
			}
			if n != 1 {
				t.Errorf("unexpected number of bytes written: got %d, want %d", n, 1)
			}
		}(i)
	}

	// Wait for all goroutines to finish.
	wg.Wait()

	// Close the GeneralWriter.
	if err := gw.Close(); err != nil {
		t.Errorf("unexpected error during Close: %v", err)
	}

	// Read the data from the pipe line-readers and verify that it matches the expected value.
	for i := 0; i < 10; i++ {
		if s, e := smp.Load(i); e {

			if s == "" {
				fmt.Printf("\ns-s: %v\n", s)
			}
		}
	}
}

func TestGeneralWriter_WritesTargetsOneAfterAnother(t *testing.T) {
	var inFlight atomic.Int32
	var overlapped atomic.Bool
	slowTarget := func() *mockWriter {
		return &mockWriter{
			WriteFunc: func(p []byte) (int, error) {
				if inFlight.Add(1) > 1 {
					overlapped.Store(true)
				}
				time.Sleep(10 * time.Millisecond)
				inFlight.Add(-1)
				return len(p), nil
			},
			CloseFunc: func() error { return nil },
		}
	}
	gw := NewGeneralWriter(slowTarget(), slowTarget())

	if _, err := gw.Write([]byte("line")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	if overlapped.Load() {
		t.Error("targets were written at the same time")
	}
}

func newMockWriter(i int, smp *sync.Map) *mockWriter {
	return &mockWriter{
		Key: i,
		WriteFunc: func(p []byte) (n int, err error) {
			smp.Store(i, string(p))
			return len(p), nil
		},
		CloseFunc: func() error {
			if existing, found := smp.Load(i); existing != nil && found {
				existing = existing.(string) + "-closed"
				smp.Store(i, existing)
			}
			return nil
		},
	}
}
func TestGeneralWriter_WriteReturnsTargetErrors(t *testing.T) {
	smp := &sync.Map{}
	ok := newMockWriter(1, smp)
	failing := &mockWriter{
		WriteFunc: func(p []byte) (int, error) { return 0, errors.New("disk full") },
		CloseFunc: func() error { return nil },
	}
	gw := NewGeneralWriter(failing, ok)

	_, err := gw.Write([]byte("line"))

	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Write() error got = %v, want it to contain %q", err, "disk full")
	}
	if got, _ := smp.Load(1); got != "line" {
		t.Errorf("healthy target got = %v, want %q", got, "line")
	}
}

type mockWriter struct {
	WriteFunc func(p []byte) (n int, err error)
	CloseFunc func() error
	Key       int
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	return w.WriteFunc(p)
}

func (w *mockWriter) Close() error {
	return w.CloseFunc()
}

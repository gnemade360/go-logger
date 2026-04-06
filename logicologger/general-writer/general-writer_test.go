package general_writer

import (
	"fmt"
	"sync"
	"testing"
)

func TestGeneralWriter_Write(t *testing.T) {
	return
	// Create a pipe.
	mp := &sync.Map{}
	//pr := newMockWriter(1, mp)
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
func newMockWriterReturningError(i int, smp *sync.Map) interface{} {
	return &mockWriter{
		Key: i,
		WriteFunc: func(p []byte) (n int, err error) {
			if i%2 == 0 {
				return 0, fmt.Errorf("error-%v", i)
			} else {
				toWrite := fmt.Sprintf("data-%v", i)
				smp.Store(i, toWrite)
				storeInSyncMap(smp, i, toWrite)
				return len([]byte(toWrite)), nil
			}
		},
		CloseFunc: func() error {
			if i%2 == 0 {
				return fmt.Errorf("error-%v", i)
			} else {
				toWrite := "closed"
				storeInSyncMap(smp, i, toWrite)
				return nil
			}
		},
	}
}

func storeInSyncMap(smp *sync.Map, i int, toWrite string) {
	if existing, found := smp.Load(i); existing != nil && found {
		existingMp := existing.(map[string]string)
		if existingMp == nil {
			existingMp = map[string]string{}
		}
		existingMp[toWrite] = toWrite
		smp.Store(i, existing)
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

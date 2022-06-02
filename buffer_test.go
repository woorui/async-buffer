package buffer

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAsyncBuffer(t *testing.T) {
	flusher := newStringCounter("", time.Microsecond)

	buf, _ := New[string](flusher, Option{Threshold: 10, FlushInterval: time.Millisecond})

	m := map[string]int{
		"AA": 100,
		"BB": 123,
		"CC": 42,
	}

	var wg sync.WaitGroup
	for k, v := range m {
		for i := 0; i < v; i++ {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				_, err := buf.Write(k)
				if err != nil {
					fmt.Println(buf.Write(k))
				}
			}(k)
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf.Flush()
	}()

	wg.Wait()

	buf.Close()

	actual := flusher.result()

	if !reflect.DeepEqual(m, actual) {
		t.Errorf("TestAsyncBuffer want: %v, actual: %v", m, actual)
	}
}

func TestWriteAfterClose(t *testing.T) {
	co := newStringCounter("", 200*time.Millisecond)

	threshold := uint32(1)

	buf, _ := New[string](co, Option{
		Threshold:     threshold,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
	})

	buf.Close()

	n, err := buf.Write("CC")

	if n != 0 || err != ErrClosed {
		t.Errorf(
			"TestWriteTimeout want: %d, %v, actual: %d, %v",
			n, err,
			0, ErrClosed,
		)
	}
}

func TestWriteTimeout(t *testing.T) {
	co := newStringCounter("", time.Hour)

	threshold := uint32(1)

	buf, _ := New[string](co, Option{
		Threshold:     threshold,
		FlushInterval: time.Hour, // block the flushing.
		WriteTimeout:  200 * time.Millisecond,
	})

	// make buffer.datas full.
	// buf.datas cap is 2 (Threshold*2), It will consume one immediately,
	// then set to 3 for making block when flush the third.
	for i := 0; i < int(threshold)*2+1; i++ {
		buf.datas <- "KK"
	}

	n, err := buf.Write("CC")

	if n != 0 || err != ErrWriteTimeout {
		t.Errorf(
			"TestWriteTimeout want: %d, %v, actual: %d, %v",
			n, err,
			0, ErrWriteTimeout,
		)
	}
}

func TestWriteDirectTimeout(t *testing.T) {
	co := newStringCounter("", time.Hour)

	buf, _ := New[string](co, Option{
		Threshold:     0, // make write direct.
		FlushInterval: 0, // make write direct.
		WriteTimeout:  200 * time.Millisecond,
	})

	// make buffer.datas full.
	// buf.datas cap is 2 (DefaultDataBackupSize*2) when Threshold is zero,
	// It will consume one immediately,
	// then set to 3 for making block when flush the third.
	for i := 0; i < DefaultDataBackupSize+1; i++ {
		buf.datas <- "KK"
	}

	n, err := buf.Write("CC")

	if n != 0 || err != ErrWriteTimeout {
		t.Errorf(
			"TestWriteDirectTimeout want: %d, %v, actual: %d, %v",
			n, err,
			0, ErrWriteTimeout,
		)
	}
}

func TestWriteDirect(t *testing.T) {
	co := newStringCounter("", 100*time.Millisecond)

	buf, _ := New[string](co, Option{
		Threshold:     0, // make write direct.
		FlushInterval: 0, // make write direct.
		WriteTimeout:  0,
	})

	// make buffer.datas full.
	// buf.datas cap is 2 (DefaultDataBackupSize*2) when Threshold is zero,
	// It will consume one immediately,
	// then set to 3 for making block when flush the third.
	for i := 0; i < DefaultDataBackupSize+1; i++ {
		buf.datas <- "KK"
	}

	n, err := buf.Write("CC", "DD", "EE", "FF")

	if n != 4 || err != nil {
		t.Errorf(
			"TestWriteDirect want: %d, %v, actual: %d, %v",
			n, err,
			0, ErrWriteTimeout,
		)
	}
}

func TestFlushFunc(t *testing.T) {
	co := newStringCounter("", time.Microsecond)

	flushfunc := co.Flush

	buf, _ := New[string](FlushFunc[string](flushfunc), Option{Threshold: 1})
	defer buf.Close()

	buf.Write("asd")
}

// stringInclude return if arr includes v
func stringInclude(arr []string, v string) bool {
	b := false
	for _, item := range arr {
		if item == v {
			b = true
			break
		}
	}
	return b
}

var errErrInput = errors.New("error input")

// stringCounter counts how many times does string appear
type stringCounter struct {
	mu            *sync.Mutex
	m             map[string]int
	errInput      string
	mockFlushCost time.Duration
}

func newStringCounter(errInput string, flushCost time.Duration) *stringCounter {
	return &stringCounter{
		mu:            &sync.Mutex{},
		m:             make(map[string]int),
		errInput:      errInput,
		mockFlushCost: flushCost,
	}
}

var k = int32(0)

func (c *stringCounter) Flush(str []string) error {
	time.Sleep(c.mockFlushCost)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range str {
		atomic.AddInt32(&k, 1)
		if v == c.errInput && c.errInput != "" {
			return errErrInput
		}
		vv := c.m[v]
		vv++
		c.m[v] = vv
	}
	return nil
}

func (c *stringCounter) result() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make(map[string]int, len(c.m))
	for k, v := range c.m {
		result[k] = v
	}
	return result
}

package buffer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestAsyncBuffer(t *testing.T) {
	flusher := newStringCounter("", time.Microsecond)

	buf := New[string](flusher, Option[string]{Threshold: 10, FlushInterval: time.Millisecond})

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
					t.Errorf("TestAsyncBuffer unexcept error: %v\n", err)
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

func TestCallFlush(t *testing.T) {
	flusher := newStringCounter("", time.Microsecond)

	buf := New[string](flusher, Option[string]{Threshold: 1000, FlushInterval: time.Hour})

	m := map[string]int{"AA": 100}

	var wg sync.WaitGroup
	for k, v := range m {
		for i := 0; i < v; i++ {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				_, err := buf.Write(k)
				if err != nil {
					fmt.Println(err)
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
		t.Errorf("TestCallFlush want: %v, actual: %v", m, actual)
	}
}

func TestWriteAfterClose(t *testing.T) {
	co := newStringCounter("", 200*time.Millisecond)

	threshold := uint32(1)

	buf := New[string](co, Option[string]{
		Threshold:     threshold,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
	})

	buf.Close()

	n, err := buf.Write("CC")

	if n != 0 || err != ErrClosed {
		t.Errorf(
			"TestWriteTimeout want: %d, %v, actual: %d, %v",
			0, ErrClosed,
			n, err,
		)
	}
}

func TestWriteTimeout(t *testing.T) {
	co := newStringCounter("", time.Hour)

	threshold := uint32(1)

	buf := New[string](co, Option[string]{
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

func TestWriteDirect(t *testing.T) {
	co := newStringCounter("", 100*time.Millisecond)

	buf := New[string](co, Option[string]{
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
			0, ErrWriteTimeout,
			n, err,
		)
	}
}

func TestWriteWithContext(t *testing.T) {
	co := newStringCounter("", 100*time.Millisecond)

	buf := New[string](co, Option[string]{})

	ctx, cancel := context.WithCancel(context.Background())

	n, err := buf.WriteWithContext(ctx, "CC", "DD", "EE", "FF")

	if n != 4 || err != nil {
		t.Errorf(
			"TestWriteWithContext before cancel ctx want: %d, %v, actual: %d, %v",
			0, nil,
			n, err,
		)
	}

	cancel()
	n, err = buf.WriteWithContext(ctx, "CC", "DD", "EE", "FF")

	if n != 0 || err != ctx.Err() {
		t.Errorf(
			"TestWriteWithContext after cancel ctx want: %d, %v, actual: %d, %v",
			0, ctx.Err(),
			n, err,
		)
	}
}

func TestWriteDirectTimeout(t *testing.T) {
	co := newStringCounter("", time.Hour)

	buf := New[string](co, Option[string]{
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
			0, ErrWriteTimeout,
			n, err,
		)
	}
}

func TestWriteDirectError(t *testing.T) {
	co := newStringCounter("ERROR", time.Millisecond)

	buf := New[string](co, Option[string]{
		Threshold:     0, // make write direct.
		FlushInterval: 0, // make write direct.
		WriteTimeout:  200 * time.Millisecond,
	})

	n, err := buf.Write("ERROR")

	if n != 0 || err != errErrInput {
		t.Errorf(
			"TestWriteDirectError want: %d, %v, actual: %d, %v",
			0, errErrInput,
			n, err,
		)
	}
}

func TestCloseError(t *testing.T) {
	co := newStringCounter("ERROR", 100*time.Millisecond)

	buf := New[string](co, Option[string]{
		Threshold:     100000,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
	})

	// make buf.datas is not empty when close.
	for i := 0; i < 100; i++ {
		buf.Write("ERROR", "ERROR", "ERROR", "ERROR")
	}

	err := buf.Close()

	if len(buf.datas) != 0 {
		if err != errErrInput {
			t.Errorf(
				"TestCloseError want: %v, actual: %v",
				errErrInput,
				err,
			)
		}
	}
}

func TestClose(t *testing.T) {
	co := newStringCounter("", 100*time.Millisecond)

	buf := New[string](co, Option[string]{
		Threshold:     100000,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
	})

	// make buf.datas is not empty when close.
	for i := 0; i < 100; i++ {
		buf.Write("AAA", "BBB", "CCC", "DDD")
	}

	err := buf.Close()

	if len(buf.datas) != 0 {
		if err != nil {
			t.Errorf(
				"TestClose want: %v, actual: %v",
				nil,
				err,
			)
		}
	}
}

func TestCloseTwice(t *testing.T) {
	co := newStringCounter("", 100*time.Millisecond)

	buf := New[string](co, Option[string]{
		Threshold:     100000,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
	})

	buf.Close()
	err := buf.Close()

	if len(buf.datas) != 0 {
		if err != nil {
			t.Errorf(
				"TestCloseTwice want: %v, actual: %v",
				ErrClosed,
				err,
			)
		}
	}
}

func TestInternalFlushFlushError(t *testing.T) {
	co := newStringCounter("ERROR", time.Millisecond)

	var ev errRecoder

	buf := New[string](co, Option[string]{
		Threshold:     10,
		FlushInterval: time.Hour,
		WriteTimeout:  200 * time.Millisecond,
		ErrHandler:    ev.log,
	})

	errElements := []string{"ERROR"}

	buf.internalFlush(errElements)

	if ev.err != errErrInput || !reflect.DeepEqual(ev.elements, errElements) {
		t.Errorf(
			"TestInternalFlushFlushError want: %v, %v, actual: %v, %v",
			errErrInput, errElements,
			ev.err, ev.elements,
		)
	}
}

func TestInternalFlushTimeout(t *testing.T) {
	co := newStringCounter("", time.Hour)

	var ev errRecoder

	buf := New[string](co, Option[string]{
		Threshold:     10,
		FlushInterval: time.Hour,
		FlushTimeout:  200 * time.Millisecond,
		ErrHandler:    ev.log,
	})

	elements := []string{"ASD"}

	buf.internalFlush(elements)

	if ev.err != ErrFlushTimeout || !reflect.DeepEqual(ev.elements, elements) {
		t.Errorf(
			"TestInternalFlushTimeout want: %v, %v, actual: %v, %v",
			errErrInput, elements,
			ev.err, ev.elements,
		)
	}
}

func TestFlushFunc(t *testing.T) {
	co := newStringCounter("", time.Microsecond)

	flushfunc := co.Flush

	buf := New[string](FlushFunc[string](flushfunc), Option[string]{Threshold: 1})
	defer buf.Close()

	buf.Write("asd")
}

func TestDefaultErrHandler(t *testing.T) {
	DefaultErrHandler(errors.New("mock_error"), []string{"A", "B", "C"})
}

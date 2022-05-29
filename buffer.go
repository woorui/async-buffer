package buffer

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultDataBackupSize is the default size of buffer data backup
const DefaultDataBackupSize = 128

var (
	// ErrClosed represents a closed buffer
	ErrClosed = errors.New("async-buffer: buffer is closed")
)

// Flusher hold FlushFunc, Flusher tell Buffer how to flush data.
type Flusher[T any] interface {
	Flush(elements []T) error
}

// The FlushFunc is an adapter to allow the use of ordinary functions
// as a Flusher. FlushFunc(f) is a Flusher that calls f.
type FlushFunc[T any] func(elements []T) error

// Flush calls f(ctx,m)
func (f FlushFunc[T]) Flush(elements []T) error {
	return f(elements)
}

// Buffer represents an async buffer
//
// The Buffer automatically flush data within a cycle
// flushing is also triggered when the data reaches the specified threshold
//
// You can also flush data manually
type Buffer[T any] struct {
	ctx        context.Context    // ctx controls the lifecycle of Buffer
	cancel     context.CancelFunc // cancel is used to stop Buffer flushing
	threshold  uint32             // flush when threshold be reached
	datas      chan T             // accept data
	doFlush    chan struct{}      // flush signal
	flusher    Flusher[T]         // flusher is interface for flushing datas element
	tickerC    <-chan time.Time   // tickerC flushs datas, when tickerC is nil, Buffer do not timed flushing
	tickerStop func()             // tickerStop stop the ticker
	errch      chan error
}

// New return the async buffer
//
// threshold indicates that the buffer is large enough to trigger flushing,
// if threshold is zero, do not judge threshold.
// flushInterval indicates the interval between automatic flushes,
// flusher is the Flusher that flushes outputs the buffer to a permanent destination.
//
// error returned is an error channel that holds errors generated during the flush process.
// You can subscribe to this channel if you want handle flush errors.
// using `se := new(buffer.ErrFlush[T]); errors.As(err, se)` to get elements that not be flushed.
func New[T any](threshold uint32, flushInterval time.Duration, flusher Flusher[T]) (*Buffer[T], <-chan error) {
	if threshold == 0 && flushInterval == 0 {
		panic(fmt.Errorf("One of threshold and flushInterval must not be 0"))
	}
	ctx, cancel := context.WithCancel(context.Background())

	var (
		tickerC    <-chan time.Time
		tickerStop func()
	)
	if flushInterval != 0 {
		tickerC, tickerStop = wrapNewTicker(flushInterval)
	}

	backupSize := DefaultDataBackupSize
	if threshold != 0 {
		backupSize = int(threshold) * 2
	}

	b := &Buffer[T]{
		ctx:        ctx,
		cancel:     cancel,
		threshold:  threshold,
		datas:      make(chan T, backupSize),
		doFlush:    make(chan struct{}, 1),
		flusher:    flusher,
		tickerC:    tickerC,
		tickerStop: tickerStop,
		errch:      make(chan error),
	}

	go b.run()

	return b, b.errch
}

// Write writes elements to buffer,
// It returns the count the written element and a closed error if buffer was closed.
func (b *Buffer[T]) Write(elements ...T) (int, error) {
	select {
	case <-b.ctx.Done():
		return 0, ErrClosed
	default:
		for _, ele := range elements {
			b.datas <- ele
		}
		return len(elements), nil
	}
}

// run do flushing in the background and send error to error channel
func (b *Buffer[T]) run() {
	flat := make([]T, 0, b.threshold)
	for {
		select {
		case <-b.ctx.Done():
			close(b.datas)
			b.flush(flat)
			return
		case d := <-b.datas:
			flat = append(flat, d)
			if b.threshold == 0 {
				continue
			}
			if len(flat) == cap(flat) {
				b.flush(flat)
				flat = flat[:0]
			}
		case <-b.doFlush:
			b.flush(flat)
			flat = flat[:0]
		case <-b.tickerC:
			b.flush(flat)
			flat = flat[:0]
		}
	}
}

func (b *Buffer[T]) flush(flat []T) {
	if len(flat) == 0 {
		return
	}
	if err := b.flusher.Flush(flat); err != nil {
		se := NewErrFlush(err, flat)
		go func() {
			b.errch <- se
		}()
		fmt.Println(se)
	}
}

// Flush flushs elements once.
func (b *Buffer[T]) Flush() { b.doFlush <- struct{}{} }

// Close stop flushing and handles rest elements.
func (b *Buffer[T]) Close() error {
	b.tickerStop()
	b.cancel()

	flat := make([]T, 0, len(b.datas))

	for v := range b.datas {
		flat = append(flat, v)
	}

	if err := b.flusher.Flush(flat); err != nil {
		se := NewErrFlush(err, flat)
		fmt.Println(se)
		return se
	}

	return nil
}

// ErrFlush is returned form `Write` when automatic flushing error,
type ErrFlush[T any] struct {
	underlying error
	Backup     []T
}

// NewErrFlush return ErrFlush, error is flush error, elements is elements that not be handled.
func NewErrFlush[T any](err error, elements []T) error {
	return ErrFlush[T]{underlying: err, Backup: elements}
}

func (e ErrFlush[T]) Error() string {
	return fmt.Sprintf("async-buffer: error while flushing error = %v, backup size = %d", e.underlying, len(e.Backup))
}

func wrapNewTicker(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(d)
	return t.C, t.Stop
}

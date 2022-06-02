package buffer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// DefaultDataBackupSize is the default size of buffer data backup
const DefaultDataBackupSize = 128

var (
	// ErrClosed represents a closed buffer
	ErrClosed = errors.New("async-buffer: buffer is closed")
	// ErrWriteTimeout returned if write timeout
	ErrWriteTimeout = errors.New("async-buffer: write timeout")
	// ErrFlushTimeout returned if flush timeout
	ErrFlushTimeout = errors.New("async-buffer: flush timeout")
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

// DefaultErrHandler.
func DefaultErrHandler[T any](err error, flat []T) {
	fmt.Fprintf(
		os.Stderr,
		"async-buffer: error while flushing error = %v, backup size = %d\n", err, len(flat))
}

// Option for New the buffer.
//
// If both Threshold and FlushInterval are set to zero, Writing is Flushing.
type Option[T any] struct {
	// Threshold indicates that the buffer is large enough to trigger flushing,
	// if Threshold is zero, do not judge threshold.
	Threshold uint32
	// WriteTimeout set write timeout, set to zero if a negative, zero means no timeout.
	WriteTimeout time.Duration
	// FlushTimeout flush timeout, set to zero if a negative, zero means no timeout.
	FlushTimeout time.Duration
	// FlushInterval indicates the interval between automatic flushes, set to zero if a negative.
	// There is automatic flushing if zero FlushInterval.
	FlushInterval time.Duration
	// ErrHandler handles errors, print error and elements size to stderr in default.
	ErrHandler func(error, []T)
}

// Buffer represents an async buffer.
//
// The Buffer automatically flush data within a cycle
// flushing is also triggered when the data reaches the specified threshold.
//
// If both Threshold and FlushInterval are set to zero, Writing is Flushing.
//
// You can also flush data manually.
type Buffer[T any] struct {
	ctx        context.Context    // ctx controls the lifecycle of Buffer
	cancel     context.CancelFunc // cancel is used to stop Buffer flushing
	datas      chan T             // accept data
	doFlush    chan struct{}      // flush signal
	tickerC    <-chan time.Time   // tickerC flushs datas, when tickerC is nil, Buffer do not timed flushing
	tickerStop func()             // tickerStop stop the ticker
	option     Option[T]          // options
	flusher    Flusher[T]         // Flusher is the Flusher that flushes outputs the buffer to a permanent destination.
	done       chan struct{}
}

// New returns the async buffer based on option
//
// error returned is an error channel that holds errors generated during the flush process.
// You can subscribe to this channel if you want handle flush errors.
// using `se := new(buffer.ErrFlush[T]); errors.As(err, &se)` to get elements that not be flushed.
func New[T any](flusher Flusher[T], option Option[T]) *Buffer[T] {
	ctx, cancel := context.WithCancel(context.Background())

	tickerC, tickerStop := wrapNewTicker(option.FlushInterval)

	backupSize := DefaultDataBackupSize
	if threshold := option.Threshold; threshold != 0 {
		backupSize = int(threshold) * 2
	}

	if option.ErrHandler == nil {
		option.ErrHandler = DefaultErrHandler[T]
	}

	b := &Buffer[T]{
		ctx:        ctx,
		cancel:     cancel,
		datas:      make(chan T, backupSize),
		doFlush:    make(chan struct{}, 1),
		tickerC:    tickerC,
		tickerStop: tickerStop,
		option:     option,
		flusher:    flusher,
		done:       make(chan struct{}, 1),
	}

	go b.run()

	return b
}

// WriteWithContext writes elements to buffer,
// It returns the count the written element and a closed error if buffer was closed.
func (b *Buffer[T]) WriteWithContext(ctx context.Context, elements ...T) (int, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}
	return b.Write(elements...)
}

// Write writes elements to buffer,
// It returns the count the written element and a closed error if buffer was closed.
func (b *Buffer[T]) Write(elements ...T) (int, error) {
	if b.option.Threshold == 0 && b.option.FlushInterval == 0 {
		return b.writeDirect(elements)
	}

	select {
	case <-b.ctx.Done():
		return 0, ErrClosed
	default:
	}
	c, stop := wrapNewTimer(b.option.WriteTimeout)
	defer stop()

	n := 0
	for _, ele := range elements {
		select {
		case <-c:
			return n, ErrWriteTimeout
		case b.datas <- ele:
			n++
		}
	}

	return n, nil
}

func (b *Buffer[T]) writeDirect(elements []T) (int, error) {
	var (
		n     = len(elements)
		errch = make(chan error, 1)
	)

	go func() {
		errch <- b.flusher.Flush(elements)
	}()

	c, stop := wrapNewTimer(b.option.WriteTimeout)
	defer stop()

	var err error
	select {
	case err = <-errch:
	case <-c:
		return 0, ErrWriteTimeout
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

// run do flushing in the background and send error to error channel
func (b *Buffer[T]) run() {
	flat := make([]T, 0, b.option.Threshold)

	for {
		select {
		case <-b.ctx.Done():
			close(b.datas)
			b.internalFlush(flat)
			b.done <- struct{}{}
			return
		case d := <-b.datas:
			flat = append(flat, d)
			if b.option.Threshold == 0 {
				continue
			}
			if len(flat) == cap(flat) {
				b.internalFlush(flat)
				flat = flat[:0]
			}
		case <-b.doFlush:
			b.internalFlush(flat)
			flat = flat[:0]
		case <-b.tickerC:
			b.internalFlush(flat)
			flat = flat[:0]
		}
	}
}

func (b *Buffer[T]) internalFlush(ts []T) {
	if len(ts) == 0 {
		return
	}

	flat := make([]T, len(ts))
	copy(flat, ts)

	done := make(chan struct{}, 1)
	go func() {
		if err := b.flusher.Flush(flat); err != nil {
			b.option.ErrHandler(err, flat)
		}
		done <- struct{}{}
	}()

	c, stop := wrapNewTimer(b.option.FlushTimeout)
	defer stop()

	select {
	case <-c:
		b.option.ErrHandler(ErrFlushTimeout, flat)
	case <-done:
	}
}

// Flush flushs elements once.
func (b *Buffer[T]) Flush() { b.doFlush <- struct{}{} }

// Close stop flushing and handles rest elements.
func (b *Buffer[T]) Close() error {
	b.cancel()
	b.tickerStop()

	<-b.done

	flat := make([]T, 0)

	for v := range b.datas {
		flat = append(flat, v)
	}

	if len(flat) == 0 {
		return nil
	}

	if err := b.flusher.Flush(flat); err != nil {
		return err
	}

	return nil
}

func wrapNewTicker(d time.Duration) (<-chan time.Time, func()) {
	var (
		c    = (<-chan time.Time)(nil)
		stop = func() {}
	)
	if d != 0 {
		t := time.NewTicker(d)
		c = t.C
		stop = t.Stop
	}

	return c, stop
}

func wrapNewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	var (
		c    = (<-chan time.Time)(nil)
		stop = func() bool { return false }
	)

	if d != 0 {
		t := time.NewTimer(d)
		c = t.C
		stop = t.Stop
	}

	return c, stop
}

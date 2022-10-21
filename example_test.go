package buffer_test

import (
	"fmt"
	"time"

	buffer "github.com/woorui/async-buffer"
)

// pp implements Flusher interface
type pp struct{}

func (p *pp) Flush(strs []string) error {
	return print(strs)
}

func print(strs []string) error {
	fmt.Printf("print: %v \n", strs)
	return nil
}

func Example() {
	// can also call buffer.FlushFunc` to adapt a function to Flusher
	buf := buffer.New[string](&pp{}, buffer.Option[string]{
		Threshold:     5,
		FlushInterval: 3 * time.Second,
		WriteTimeout:  time.Second,
		FlushTimeout:  time.Second,
		ErrHandler:    func(err error, t []string) { fmt.Printf("err: %v, ele: %v", err, t) },
	})
	// data maybe loss if Close() is not be called
	defer buf.Close()

	buf.Write("a", "b", "c", "d", "e", "f")

	// Output:
	// 	print: [a b c d e f]
}

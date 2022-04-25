# async-buffer

[![Go](https://github.com/woorui/async-buffer/actions/workflows/go.yml/badge.svg)](https://github.com/woorui/async-buffer/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/woorui/async-buffer/branch/main/graph/badge.svg?token=G7OK0KG9YT)](https://codecov.io/gh/woorui/async-buffer)

The async-buffer buffer data that can be flushed when reach threshold or duration limit. It is multi-goroutinue safe.

**It only support go1.18 or later**

## Why you need it?

### An Usecase: 

You have a message queue subscriber server.

The Server receive message one by one and insert then your database,

But there is a big performance gap that between one by one insertion and batch insertion to your database.

So that to use async-buffer to buffer data then find a timing to batch insert them.

## Installation

```
go get -u github.com/woorui/async-buffer
```

## Quick start

```go
package main

import (
	"errors"
	"fmt"
	"time"

	buffer "github.com/woorui/async-buffer"
)

type printer struct{}

func (p *printer) Flush(strs ...string) error {
	fmt.Printf("printer flush elements: %v, flush size: %d \n", strs, len(strs))
	return nil
}

func main() {
	buf, errch := buffer.New[string](6, 3*time.Second, &printer{})

	// If you don't care about the refresh error
	// and the refresh error elements, you can ignore them.
	go errHandle(errch)

	// 1. flush at threshold
	buf.Write("a", "b", "c", "d", "e", "f")
	// Output
	// printer flush elements: [a b c d e f], flush size: 6

	// 2. time to flush automatically
	buf.Write("aaaaa")
	buf.Write("bbbbb")
	buf.Write("ccccc", "ddddd")
	time.Sleep(5 * time.Second)
	// Output
	// printer flush elements: [aaaaa bbbbb ccccc ddddd], flush size: 4

	// 3. flush manually
	buf.Write("eeeee", "fffff")
	buf.Flush()
	// Output
	// printer flush elements: [eeeee fffff], flush size: 2

	// waiting...
	select {}
}

func errHandle(errch <-chan error) {
	for {
		select {
		case err := <-errch:
			fmt.Printf("flush err %v \n", err)
			if se := new(buffer.ErrFlush[string]); errors.As(err, se) {
				fmt.Printf("flush err backup %v \n", se.Backup)
			}
		}
	}
}

```

## License

[MIT License](https://github.com/woorui/async-buffer/blob/main/LICENSE)

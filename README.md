# async-buffer

[![Go](https://github.com/woorui/async-buffer/actions/workflows/go.yml/badge.svg)](https://github.com/woorui/async-buffer/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/woorui/async-buffer/branch/main/graph/badge.svg?token=G7OK0KG9YT)](https://codecov.io/gh/woorui/async-buffer)

The async-buffer buffer data that can be flushed when reach threshold or duration limit. It is multi-goroutinue safe.

**It only support go1.18 or later**

## Why you need it?

### An Usecase: 

You have a message queue subscriber server.

The Server receives messages one by one and inserts them into your database,

But there is a big performance gap between one by one insertion and batch insertion to your database.

So that to use async-buffer to buffer data then find timing to batch insert them.

## Installation

```
go get -u github.com/woorui/async-buffer
```

## Documents

Complete doc here: https://pkg.go.dev/github.com/woorui/async-buffer

## Quick start

The `Write`, `Flush`, `Close` api are goroutinue-safed.

```go
package main

import (
	"context"
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

func main() {
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

	// 1. flush at threshold
	buf.Write("a", "b", "c", "d", "e", "f")
	// Output
	// print: [a b c d e f]

	// 2. time to flush automatically
	buf.Write("aaaaa")
	buf.Write("bbbbb")
	buf.Write("ccccc", "ddddd")
	time.Sleep(5 * time.Second)
	// Output
	// print: [aaaaa bbbbb ccccc ddddd]

	// 3. flush manually and write call `WriteWithContext`
	buf.WriteWithContext(context.Background(), "eeeee", "fffff")
	buf.Flush()
	// Output
	// print: [eeeee fffff]
}

```

## License

[MIT License](https://github.com/woorui/async-buffer/blob/main/LICENSE)

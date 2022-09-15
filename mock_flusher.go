package buffer

import (
	"errors"
	"sync"
	"time"
)

var errErrInput = errors.New("error input")

// stringCounter counts how many times does string appears.
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

// Flush implements Flusher interface.
func (c *stringCounter) Flush(str []string) error {
	time.Sleep(c.mockFlushCost)
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, v := range str {
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

// errValidater records err and err elements
type errValidater struct {
	err      error
	elements []string
}

func (ev *errValidater) log(err error, elements []string) {
	ev.err = err
	ev.elements = elements
}

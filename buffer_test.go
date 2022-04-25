package buffer

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestAsyncBuffer(t *testing.T) {
	type args struct {
		errInput       string        // counter error when errInput is not empty
		dataGroup      [][]string    // group data be writen to async buffer
		flushThreshold uint32        // data flush when flushThreshold be reached
		mockFlushCost  time.Duration // time cost for flushing
		flushInterval  time.Duration // flush interval
		waitDuration   time.Duration // wait flushing duration
		manual         bool          // manual flushing, only one of manual and manual is true
		close          bool          // close flushing, only one of manual and manual is true
	}
	tests := []struct {
		name            string
		args            args
		skipCompareWant bool
		want            map[string]int
		wantErrBackup   []string
	}{
		{
			name: "threshold be reched flushing",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{{"AA", "BB"}, {"AA"}, {"DD"}},
				flushThreshold: 4,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Hour,
				waitDuration:   time.Second,
			},
			want: map[string]int{"AA": 2, "BB": 1, "DD": 1},
		},
		{
			name: "time to flushing",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{{"AA", "BB"}, {"AA"}, {"DD"}, {"EEEEE"}},
				flushThreshold: 0,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Second,
				waitDuration:   2 * time.Second,
			},
			want: map[string]int{"AA": 2, "BB": 1, "DD": 1, "EEEEE": 1},
		},
		{
			name: "flush manually",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{{"EEEEE"}},
				flushThreshold: 32,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Hour,
				waitDuration:   2 * time.Second,
				manual:         true,
			},
			want: map[string]int{"EEEEE": 1},
		},
		{
			name: "close immediately",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{{"AA", "BB"}, {"AA"}, {"DD"}, {"FFF"}},
				flushThreshold: 32,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Hour,
				waitDuration:   time.Second,
				close:          true,
			},
			want: map[string]int{"AA": 2, "BB": 1, "DD": 1, "FFF": 1},
		},
		{
			name: "flushThreshold is zero",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{},
				mockFlushCost:  time.Microsecond,
				flushThreshold: 0,
				flushInterval:  time.Hour,
			},
			want: map[string]int{},
		},
		{
			name: "subscribe the error",
			args: args{
				errInput:       "AA",
				dataGroup:      [][]string{{"AA"}},
				flushThreshold: 1,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Millisecond,
				waitDuration:   time.Second,
				manual:         true,
				close:          true,
			},
			want:          map[string]int{},
			wantErrBackup: []string{"AA"},
		},
		{
			name: "closing buffer with rest data",
			args: args{
				errInput:       "",
				dataGroup:      [][]string{{"BB"}, {"AA"}, {"BB", "BB", "CC"}},
				flushThreshold: 1,
				mockFlushCost:  time.Millisecond,
				flushInterval:  time.Microsecond,
				waitDuration:   time.Second,
				manual:         false,
				close:          true,
			},
			want:          map[string]int{"BB": 3, "CC": 1, "AA": 1},
			wantErrBackup: []string{"AA"},
		},
		{
			name: "closing buffer with all data error",
			args: args{
				errInput:       "AA",
				dataGroup:      [][]string{{"AA"}, {"AA"}, {"AA", "AA", "AA"}},
				flushThreshold: 1,
				mockFlushCost:  time.Microsecond,
				flushInterval:  time.Hour,
				waitDuration:   time.Second,
				manual:         false,
				close:          true,
			},
			skipCompareWant: true,
			want:            map[string]int{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := newStringCounter(tt.args.errInput, tt.args.mockFlushCost)

			buf, errch := New[string](tt.args.flushThreshold, tt.args.flushInterval, co)

			var wg sync.WaitGroup
			for _, group := range tt.args.dataGroup {
				wg.Add(1)
				go func(group []string) {
					defer wg.Done()
					includes := stringInclude(group, tt.args.errInput)

					n, err := buf.Write(group...)
					if err != nil && !includes {
						t.Errorf("Not expected error occurred while testing Write in multiple goroutinue, testdata = %+v", group)
					}
					if n != len(group) && !includes {
						t.Errorf("Write size error in multiple goroutinue, testdata = %+v, return len = %d", group, n)
					}
				}(group)
			}
			wg.Wait()

			if tt.args.manual {
				buf.Flush()
			} else if tt.args.close {
				buf.Close()
				_, err := buf.Write("something")
				if !errors.Is(err, ErrClosed) {
					t.Errorf("Write after close should return ErrClosed")
				}
			}

			if tt.args.waitDuration != 0 {
				time.Sleep(tt.args.waitDuration)
			}

			result := co.result()

			if !tt.skipCompareWant {
				if !reflect.DeepEqual(result, tt.want) {
					t.Errorf("Flush result error, actual: %v, want: %v", result, tt.want)
				}
			} else {
				t.Logf("result: %v", result)
			}

			select {
			case err := <-errch:
				t.Log(err)
				if se := new(ErrFlush[string]); errors.As(err, se) {
					if !tt.skipCompareWant && !reflect.DeepEqual(se.backup, tt.wantErrBackup) {
						t.Errorf("Returns err backup error, actual: %v, want: %v", se.backup, tt.wantErrBackup)
					}
				}
			default:
			}

		})
	}
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

func (c *stringCounter) Flush(str ...string) error {
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

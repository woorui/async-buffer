package buffer

import (
	"reflect"
	"testing"
	"time"
)

func Test_newStringCounter(t *testing.T) {
	type args struct {
		errInput  string
		flushCost time.Duration
		elements  []string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]int
		wantErr error
	}{
		{
			name: "normal",
			args: args{
				errInput:  "",
				flushCost: time.Millisecond,
				elements:  []string{"A", "B", "C", "D", "D"},
			},
			want:    map[string]int{"A": 1, "B": 1, "C": 1, "D": 2},
			wantErr: nil,
		},
		{
			name: "error",
			args: args{
				errInput:  "A",
				flushCost: time.Millisecond,
				elements:  []string{"A", "B", "C", "D", "D"},
			},
			want:    map[string]int{},
			wantErr: errErrInput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			co := newStringCounter(tt.args.errInput, tt.args.flushCost)

			gotErr := co.Flush(tt.args.elements)

			if gotErr != tt.wantErr {
				t.Errorf("stringCounter got error = %v, want error %v", gotErr, tt.wantErr)
			}

			got := co.result()

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("stringCounter got = %v, want %v", got, tt.want)
			}
		})
	}
}

package bandit

import (
	"testing"
)

func Test_normalizeReceiveSpeed(t *testing.T) {
	type args struct {
		dataRecv uint64
	}
	tests := []struct {
		name string
		args args
		want func(float64) bool
	}{
		{
			name: "should return 0 if no data received",
			args: args{
				dataRecv: 0,
			},
			want: func(got float64) bool {
				return got == 0
			},
		},
		{
			name: "should return 1 if pretty fast",
			args: args{
				dataRecv: topExpectedSpeed * secondsForSample,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},
		{
			name: "should return 1 if super fast",
			args: args{
				dataRecv: topExpectedSpeed * 50,
			},
			want: func(got float64) bool {
				return got == 1
			},
		},

		{
			name: "should return <1 if sorta fast",
			args: args{
				dataRecv: 2000,
			},
			want: func(got float64) bool {
				return got > 0 && got < 1
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeReceiveSpeed(tt.args.dataRecv); !tt.want(got) {
				t.Errorf("unexpected normalizeReceiveSpeed() = %v", got)
			}
		})
	}
}

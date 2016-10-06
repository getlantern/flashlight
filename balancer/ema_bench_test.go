package balancer

/*
Not a benchmark actually. It's just to give us an sense how different
calculation of "average" looks like (Imagine the data points are random connect
time of an 500ms link vs 200ms link). Example result:

	ema_bench_test.go:53: d:702	avg:509	α=0.5: 732	α=0.2: 652	α=0.1: 603	||	d:98	avg:194	α=0.5: 173	α=0.2: 192	α=0.1: 189
	ema_bench_test.go:53: d:143	avg:509	α=0.5: 437	α=0.2: 550	α=0.1: 557	||	d:38	avg:194	α=0.5: 105	α=0.2: 161	α=0.1: 173
	ema_bench_test.go:53: d:930	avg:509	α=0.5: 683	α=0.2: 626	α=0.1: 594	||	d:232	avg:194	α=0.5: 168	α=0.2: 175	α=0.1: 178
	ema_bench_test.go:53: d:409	avg:509	α=0.5: 546	α=0.2: 582	α=0.1: 575	||	d:320	avg:194	α=0.5: 244	α=0.2: 204	α=0.1: 192
	ema_bench_test.go:53: d:623	avg:509	α=0.5: 584	α=0.2: 590	α=0.1: 579	||	d:112	avg:194	α=0.5: 178	α=0.2: 185	α=0.1: 184
	ema_bench_test.go:53: d:778	avg:509	α=0.5: 681	α=0.2: 627	α=0.1: 598	||	d:198	avg:194	α=0.5: 188	α=0.2: 187	α=0.1: 185
	ema_bench_test.go:53: d:355	avg:509	α=0.5: 518	α=0.2: 572	α=0.1: 573	||	d:68	avg:194	α=0.5: 128	α=0.2: 163	α=0.1: 173
	ema_bench_test.go:53: d:694	avg:509	α=0.5: 606	α=0.2: 596	α=0.1: 585	||	d:85	avg:194	α=0.5: 106	α=0.2: 147	α=0.1: 164
	ema_bench_test.go:53: d:379	avg:509	α=0.5: 492	α=0.2: 552	α=0.1: 564	||	d:70	avg:194	α=0.5: 88	α=0.2: 131	α=0.1: 154
	ema_bench_test.go:53: d:517	avg:509	α=0.5: 504	α=0.2: 545	α=0.1: 559	||	d:56	avg:194	α=0.5: 72	α=0.2: 116	α=0.1: 144
*/

import (
	"math/rand"
	"testing"
	"time"
)

func BenchmarkEMA(b *testing.B) {
	total_A := 0
	ema_alpha_0_5_A := &emaDuration{ia: 2}
	ema_alpha_0_2_A := &emaDuration{ia: 5}
	ema_alpha_0_1_A := &emaDuration{ia: 10}

	total_B := 0
	ema_alpha_0_5_B := &emaDuration{ia: 2}
	ema_alpha_0_2_B := &emaDuration{ia: 5}
	ema_alpha_0_1_B := &emaDuration{ia: 10}

	for i := 0; i < b.N; i++ { //use b.N for looping
		d_A := rand.Intn(1000)
		total_A = total_A + d_A
		ema_alpha_0_5_A.UpdateWith(time.Duration(d_A))
		ema_alpha_0_2_A.UpdateWith(time.Duration(d_A))
		ema_alpha_0_1_A.UpdateWith(time.Duration(d_A))

		d_B := rand.Intn(400)
		total_B = total_B + d_B
		ema_alpha_0_5_B.UpdateWith(time.Duration(d_B))
		ema_alpha_0_2_B.UpdateWith(time.Duration(d_B))
		ema_alpha_0_1_B.UpdateWith(time.Duration(d_B))

		if i > 1000 {
			b.Logf("d:%d\tavg:%d\tα=0.5: %d\tα=0.2: %d\tα=0.1: %d\t||\td:%d\tavg:%d\tα=0.5: %d\tα=0.2: %d\tα=0.1: %d",
				d_A, total_A/(i+1), ema_alpha_0_5_A.GetInt64(), ema_alpha_0_2_A.GetInt64(), ema_alpha_0_1_A.GetInt64(),
				d_B, total_B/(i+1), ema_alpha_0_5_B.GetInt64(), ema_alpha_0_2_B.GetInt64(), ema_alpha_0_1_B.GetInt64())
		}
	}
}

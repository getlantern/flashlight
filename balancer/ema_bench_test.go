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
	totalA := 0
	emaAlpha05A := &ema{defaultAlpha: 0.5}
	emaAlpha02A := &ema{defaultAlpha: 0.2}
	emaAlpha01A := &ema{defaultAlpha: 0.1}

	totalB := 0
	emaAlpha05B := &ema{defaultAlpha: 0.5}
	emaAlpha02B := &ema{defaultAlpha: 0.2}
	emaAlpha01B := &ema{defaultAlpha: 0.1}

	for i := 0; i < b.N; i++ { //use b.N for looping
		dA := rand.Intn(1000)
		totalA = totalA + dA
		emaAlpha05A.updateDuration(time.Duration(dA))
		emaAlpha02A.updateDuration(time.Duration(dA))
		emaAlpha01A.updateDuration(time.Duration(dA))

		dB := rand.Intn(400)
		totalB = totalB + dB
		emaAlpha05B.updateDuration(time.Duration(dB))
		emaAlpha02B.updateDuration(time.Duration(dB))
		emaAlpha01B.updateDuration(time.Duration(dB))

		if i > 1000 {
			b.Logf("d:%d\tavg:%d\tα=0.5: %.0f\tα=0.2: %.0f\tα=0.1: %.0f\t||\td:%d\tavg:%d\tα=0.5: %.0f\tα=0.2: %.0f\tα=0.1: %.0f",
				dA, totalA/(i+1), emaAlpha05A.get(), emaAlpha02A.get(), emaAlpha01A.get(),
				dB, totalB/(i+1), emaAlpha05B.get(), emaAlpha02B.get(), emaAlpha01B.get())
		}
	}
}

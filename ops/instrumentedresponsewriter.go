package ops

import (
	"fmt"
	"net/http"
	"time"
)

type InstrumentedResponseWriter struct {
	http.ResponseWriter
	Op      *Op
	start   time.Time
	label   string
	written int
}

func InitInstrumentedResponseWriter(w http.ResponseWriter, label string) *InstrumentedResponseWriter {
	op := Begin(label)
	start := time.Now()

	return &InstrumentedResponseWriter{
		w,
		op,
		start,
		label,
		0,
	}
}

func (w *InstrumentedResponseWriter) Finish() {
	totalTime := time.Now().Sub(w.start).Milliseconds()
	w.Op.SetMetricPercentile(fmt.Sprintf("%s_response_time_ms", w.label), float64(totalTime))
	rate := float64(w.written/int(totalTime)) / 125 // bytes/ms -> mbps
	w.Op.SetMetricPercentile(fmt.Sprintf("%s_response_rate_mbps", w.label), rate)
	w.Op.SetMetricSum(fmt.Sprintf("%s_bytes", w.label), float64(w.written))
	w.Op.End()
}

func (w *InstrumentedResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *InstrumentedResponseWriter) Write(p []byte) (n int, err error) {
	timeBeforeWrite := time.Now()

	written, err := w.ResponseWriter.Write(p)

	if written > 0 && w.written == 0 {
		w.Op.SetMetricPercentile(fmt.Sprintf("%s_first_byte_ms", w.label), float64(timeBeforeWrite.Sub(w.start).Milliseconds()))
	}

	w.written += written
	return written, err
}

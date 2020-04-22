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
	totalTime := time.Now().Sub(w.start).Seconds()
	w.Op.SetMetricPercentile(fmt.Sprintf("%s_response_time", w.label), totalTime)
	rate := float64(w.written) / totalTime
	w.Op.SetMetricPercentile(fmt.Sprintf("%s_response_rate", w.label), rate)
	w.Op.End()
}

func (w *InstrumentedResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.Op.Set("status", statusCode)
	w.Op.SetMetricPercentile(fmt.Sprintf("%s_header_time", w.label), time.Now().Sub(w.start).Seconds())
}

func (w *InstrumentedResponseWriter) Write(p []byte) (n int, err error) {
	written, err := w.ResponseWriter.Write(p)
	w.written += written
	return written, err
}

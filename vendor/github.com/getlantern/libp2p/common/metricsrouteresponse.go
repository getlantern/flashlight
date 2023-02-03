package common

import "fmt"

// MetricsRouteResponse is the payload that's returned from a FreePeer to
// show the performance of its proxy. Mostly bytes read and written.
type MetricsRouteResponse struct {
	TotalBytesRead    int64 `json:"total-bytes-read"`
	TotalBytesWritten int64 `json:"total-bytes-written"`
}

func (r *MetricsRouteResponse) String() string {
	return fmt.Sprintf(
		"totalBytesRead=%d, totalBytesWritten=%d",
		r.TotalBytesRead,
		r.TotalBytesWritten,
	)
}

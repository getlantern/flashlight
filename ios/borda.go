package ios

import (
	"os"
	"sync"

	bclient "github.com/getlantern/borda/client"
	"github.com/getlantern/msgpack"

	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/ops"
)

var (
	initOnce sync.Once
)

type row struct {
	Values     map[string]bclient.Val
	Dimensions map[string]interface{}
}

// ConfigureBorda configures borda for capturing metrics on iOS
func ConfigureBorda(deviceID string, samplePercentage float64, bufferFile string) (finalErr error) {
	initOnce.Do(func() {
		ops.InitGlobalContext(deviceID, func() bool { return false }, func() int64 { return 0 }, func() string { return "" }, func() string { return "" })
		bf, err := os.OpenFile(bufferFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			finalErr = err
			return
		}
		out := msgpack.NewEncoder(bf)
		borda.ConfigureWithSubmitter(func(values map[string]bclient.Val, dims map[string]interface{}) error {
			return out.Encode(&row{Values: values, Dimensions: dims})
		}, borda.Enabler(samplePercentage))
	})

	return finalErr
}

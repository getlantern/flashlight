package ios

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	bclient "github.com/getlantern/borda/client"
	"github.com/getlantern/fronted"
	"github.com/getlantern/msgpack"

	"github.com/getlantern/flashlight/borda"
	"github.com/getlantern/flashlight/ops"
)

var (
	initOnce sync.Once

	forceFlush = make(chan bool, 0)
)

type row struct {
	Values     map[string]bclient.Val
	Dimensions map[string]interface{}
}

func init() {
	msgpack.RegisterExt(10, &row{})
}

// ConfigureBorda configures borda for capturing metrics on iOS and starts a
// process that buffers recorded metrics to disk for use by ReportToBorda.
func ConfigureBorda(deviceID string, samplePercentage float64, bufferFlushInterval string, bufferFile, tempBufferFile string) (finalErr error) {
	initOnce.Do(func() {
		ops.InitGlobalContext(deviceID, func() bool { return false }, func() int64 { return 0 }, func() string { return "" }, func() string { return "" })

		var bf *os.File
		var err error
		var out *msgpack.Encoder

		openTempBuffer := func() {
			bf, err = os.OpenFile(tempBufferFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				panic(fmt.Errorf("unable to open temp buffer file %v: %v", tempBufferFile, err))
			}
			out = msgpack.NewEncoder(bf)
		}

		openTempBuffer()
		var flushMx sync.Mutex
		lastFlushed := time.Now()

		flushBufferIf := func(necessary func() bool) {
			flushMx.Lock()
			defer flushMx.Unlock()
			if necessary() {
				log.Debugf("Flushing temporary buffer %v to %v", tempBufferFile, bufferFile)
				if err := os.Rename(tempBufferFile, bufferFile); err != nil {
					log.Errorf("unable to rename temp buffer file %v to %v, will discard buffered data: %v", tempBufferFile, bufferFile)
				}
				openTempBuffer()
				lastFlushed = time.Now()
			}
		}

		flushInterval, err := time.ParseDuration(bufferFlushInterval)
		if err != nil {
			finalErr = err
			return
		}
		flushBufferIfNecessary := func() {
			flushBufferIf(func() bool { return time.Now().Sub(lastFlushed) > flushInterval })
		}

		go func() {
			for range forceFlush {
				flushBufferIf(func() bool { return true })
			}
		}()

		borda.ConfigureWithSubmitter(func(values map[string]bclient.Val, dims map[string]interface{}) error {
			if err := out.Encode(&row{Values: values, Dimensions: dims}); err != nil {
				return err
			}
			flushBufferIfNecessary()
			return nil
		}, borda.Enabler(samplePercentage))
	})

	return finalErr
}

// ReportToBorda reports buffered metrics to borda
func ReportToBorda(bufferFile string) error {
	rt, ok := fronted.NewDirect(1 * time.Minute)
	if !ok {
		return fmt.Errorf("Timed out waiting for fronting to finish configuring")
	}

	bordaClient := bclient.NewClient(&bclient.Options{
		BatchInterval: 10000 * time.Hour, // don't use on automated flush
		HTTPClient: &http.Client{
			Transport: rt,
		},
	})
	defer bordaClient.Flush()

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)

	defer os.Remove(bufferFile)
	file, err := os.OpenFile(bufferFile, os.O_RDONLY, 0644)
	if err != nil {
		return fmt.Errorf("unable to borda open buffer file %v: %v", bufferFile, err)
	}

	dec := msgpack.NewDecoder(file)
	for {
		_row, err := dec.DecodeInterface()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("unable to decode row from borda buffer file %v: %v", bufferFile, err)
		}
		row, ok := _row.(*row)
		if !ok {
			return fmt.Errorf("unexpected type of row value from borda buffer file %v: %v", bufferFile, reflect.TypeOf(_row))
		}

		reportToBorda(row.Values, row.Dimensions)
	}
}

// ForceFlush forces a flush
func ForceFlush() {
	select {
	case forceFlush <- true:
		log.Debug("Requested flush")
	default:
		// already pending, ignore
	}
}

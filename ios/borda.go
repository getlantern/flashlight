package ios

import (
	"bufio"
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

	forceFlush = make(chan chan interface{}, 0)
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
// ConfigureBorda reads configuration from disk and relies on Configure having
// been called first.
func ConfigureBorda(configDir, bufferFile, tempBufferFile string) (finalErr error) {
	initOnce.Do(func() {
		cf := &configurer{configFolderPath: configDir}
		global, _, _, err := cf.openGlobal()
		if err != nil {
			finalErr = fmt.Errorf("Unable to read global config: %v", err)
			return
		}
		uc, err := cf.readUserConfig()
		if err != nil {
			finalErr = fmt.Errorf("Unable to read user config: %v", err)
			return
		}
		ops.InitGlobalContext(uc.DeviceID, func() bool { return false }, func() int64 { return 0 }, func() string { return "" }, func() string { return "" })

		samplePercentage := global.BordaSamplePercentage
		flushInterval := global.BordaReportInterval
		log.Debugf("Configuring borda with sample percentage %v and flush interval %v", samplePercentage, flushInterval)

		var bf *os.File
		var buf *bufio.Writer
		var out *msgpack.Encoder

		openTempBuffer := func() {
			bf, err = os.OpenFile(tempBufferFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				finalErr = log.Errorf("unable to open temp buffer file %v: %v", tempBufferFile, err)
				out = nil
				return
			}
			buf = bufio.NewWriter(bf)
			out = msgpack.NewEncoder(buf)
		}

		openTempBuffer()
		var flushMx sync.Mutex
		lastFlushed := time.Now()

		flushBufferIf := func(necessary func() bool) bool {
			flushMx.Lock()
			defer flushMx.Unlock()
			if necessary() {
				if out != nil {
					log.Debugf("Flushing temporary buffer %v to %v", tempBufferFile, bufferFile)
					if err := buf.Flush(); err != nil {
						log.Errorf("Error flushing buffered writes to %v: %v", tempBufferFile, err)
					}
					if err := bf.Close(); err != nil {
						log.Errorf("Error closing encoder on %v: %v", tempBufferFile, err)
					}
					if err := os.Rename(tempBufferFile, bufferFile); err != nil {
						log.Errorf("unable to rename temp buffer file %v to %v, will discard buffered data: %v", tempBufferFile, bufferFile)
					}
				}
				openTempBuffer()
				lastFlushed = time.Now()
				return true
			}
			return false
		}

		flushBufferIfNecessary := func() {
			flushBufferIf(func() bool { return time.Now().Sub(lastFlushed) > flushInterval })
		}

		go func() {
			for flushed := range forceFlush {
				if flushBufferIf(func() bool { return true }) {
					close(flushed)
				}
			}
		}()

		borda.ConfigureWithSubmitter(func(values map[string]bclient.Val, dims map[string]interface{}) error {
			flushMx.Lock()
			o := out
			flushMx.Unlock()
			if o != nil {
				if err := o.Encode(&row{Values: values, Dimensions: dims}); err != nil {
					return err
				}
			}
			flushBufferIfNecessary()
			return nil
		}, borda.Enabler(samplePercentage))
	})

	return finalErr
}

// ReportToBorda reports buffered metrics to borda
func ReportToBorda(bufferFile string) error {
	hasReportedSomeData := false
	defer func() {
		if hasReportedSomeData {
			os.Remove(bufferFile)
		}
	}()

	file, err := os.OpenFile(bufferFile, os.O_RDONLY, 0644)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("unable to open borda buffer file %v: %v", bufferFile, err)
	}
	defer file.Close()

	rt, ok := fronted.NewDirect(1 * time.Minute)
	if !ok {
		return fmt.Errorf("Timed out waiting for fronting to finish configuring")
	}

	bordaClient := bclient.NewClient(&bclient.Options{
		BatchInterval: 10000 * time.Hour, // don't use on automated flush
		HTTPClient: &http.Client{
			Transport: rt,
			Timeout:   3 * time.Minute,
		},
	})
	defer bordaClient.Flush()

	reportToBorda := bordaClient.ReducingSubmitter("client_results", 1000)
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

		if err := reportToBorda(row.Values, row.Dimensions); err != nil {
			return err
		}

		hasReportedSomeData = true
	}
}

// ForceFlush forces a flush and waits up to 15 seconds for the flush to finish
func ForceFlush() {
	flushed := make(chan interface{}, 0)
	select {
	case forceFlush <- flushed:
		select {
		case <-flushed:
			return
		case <-time.After(15 * time.Second):
			return
		}
	default:
		// already pending, ignore
	}
}

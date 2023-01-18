package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/getlantern/errors"
	"github.com/getlantern/golog"
	"github.com/getlantern/ops"
	"github.com/oxtoacart/bpool"
)

var (
	log = golog.LoggerFor("borda.client")

	bufferPool = bpool.NewBufferPool(100)

	errDiscard = errors.New("Exceeded max buffer size, discarding measurement")
)

// DefaultURL is the URL used if Options.URL is not set.
const DefaultURL = "https://borda.lantern.io/measurements"

// Measurement represents a measurement at a point in time.
type Measurement struct {
	// Name is the name of the measurement (e.g. cpu_usage).
	Name string `json:"name"`

	// Ts records the time of the measurement.
	Ts time.Time `json:"ts,omitempty"`

	// Values contains numeric values of the measurement.
	//
	// Example: { "num_errors": 67 }
	Values map[string]Val `json:"values,omitempty"`

	// Dimensions captures key/value pairs which characterize the measurement.
	//
	// Example: { "requestid": "18af517b-004f-486c-9978-6cf60be7f1e9",
	//            "ipv6": "2001:0db8:0a0b:12f0:0000:0000:0000:0001",
	//            "host": "myhost.mydomain.com",
	//            "total_cpus": "2",
	//            "cpu_idle": 10.1,
	//            "cpu_system": 53.3,
	//            "cpu_user": 36.6,
	//            "connected_to_internet": true }
	Dimensions json.RawMessage `json:"dimensions,omitempty"`

	TypedDimensions map[string]interface{} `json:"-"`
}

// Options provides configuration options for borda clients
type Options struct {
	// BatchInterval specifies how frequent to report to borda
	BatchInterval time.Duration

	// HTTP Client used to report to Borda
	HTTPClient *http.Client

	// Sender, if specified, is used in place of the default HTTPClient
	Sender func(batch map[string][]*Measurement) (int, error)

	// BeforeSubmit is an optional callback that gets called before submitting a
	// batch to borda. The callback should not modify the values and dimensions.
	BeforeSubmit func(name string, ts time.Time, values map[string]Val, dimensionsJSON []byte)

	// URL to target. Defaults to DefaultURL.
	URL string
}

func (o Options) url() string {
	if o.URL == "" {
		return DefaultURL
	}
	return o.URL
}

// Submitter is a functon that submits measurements to borda. If the measurement
// was successfully queued for submission, this returns nil.
type Submitter func(values map[string]Val, dimensions map[string]interface{}) error

type submitter func(key string, ts time.Time, values map[string]Val, dimensions map[string]interface{}, jsonDimensions []byte) error

// Client is a client that submits measurements to the borda server.
type Client struct {
	sender              func(batch map[string][]*Measurement) (int, error)
	options             *Options
	buffers             map[int]map[string]*Measurement
	submitters          map[int]submitter
	nextBufferID        int
	bytesSent           int
	changeBatchInterval chan time.Duration
	mx                  sync.Mutex
}

// NewClient creates a new borda client.
func NewClient(opts *Options) *Client {
	if opts == nil {
		opts = &Options{}
	}
	if opts.BatchInterval <= 0 {
		log.Debugf("BatchInterval has to be greater than zero, defaulting to 5 minutes")
		opts.BatchInterval = 5 * time.Minute
	}
	if opts.HTTPClient == nil {
		// Use a default HTTP client
		opts.HTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					ClientSessionCache: tls.NewLRUClientSessionCache(100),
				},
			},
		}
	}

	if opts.BeforeSubmit == nil {
		opts.BeforeSubmit = func(name string, ts time.Time, values map[string]Val, dimensionsJSON []byte) {
		}
	}

	c := &Client{
		sender:              opts.Sender,
		options:             opts,
		buffers:             make(map[int]map[string]*Measurement),
		submitters:          make(map[int]submitter),
		changeBatchInterval: make(chan time.Duration, 100),
	}

	if c.sender == nil {
		// Default to sending with HTTP
		c.sender = c.buildHTTPSender(opts.HTTPClient)
	}

	go c.sendPeriodically(opts.BatchInterval)
	return c
}

// EnableOpsReporting registers a reporter with the ops package that reports op
// successes and failures to borda under the given measurement name.
func (c *Client) EnableOpsReporting(name string, maxBufferSize int) {
	reportToBorda := c.ReducingSubmitter(name, maxBufferSize)

	ops.RegisterReporter(func(failure error, ctx map[string]interface{}) {
		values := map[string]Val{}
		if failure != nil {
			values["error_count"] = Float(1)
		} else {
			values["success_count"] = Float(1)
		}

		reportErr := reportToBorda(values, ctx)
		if reportErr != nil {
			log.Errorf("Error reporting error to borda: %v", reportErr)
		}
	})
}

// ReducingSubmitter returns a Submitter whose measurements are reduced based on
// their types. name specifies the name of the measurements and
// maxBufferSize specifies the maximum number of distinct measurements to buffer
// within the BatchInterval. Anything past this is discarded.
func (c *Client) ReducingSubmitter(name string, maxBufferSize int) Submitter {
	if maxBufferSize <= 0 {
		log.Debugf("maxBufferSize has to be greater than zero, defaulting to 1000")
		maxBufferSize = 1000
	}
	c.mx.Lock()
	defer c.mx.Unlock()
	bufferID := c.nextBufferID
	c.nextBufferID++
	submitter := func(key string, ts time.Time, values map[string]Val, dimensions map[string]interface{}, jsonDimensions []byte) error {
		buffer := c.buffers[bufferID]
		if buffer == nil {
			// Lazily initialize buffer
			buffer = make(map[string]*Measurement)
			c.buffers[bufferID] = buffer
		}
		existing, found := buffer[key]
		if found {
			for key, value := range values {
				existing.Values[key] = value.Merge(existing.Values[key])
			}
			if ts.After(existing.Ts) {
				existing.Ts = ts
			}
		} else if len(buffer) == maxBufferSize {
			return errDiscard
		} else {
			buffer[key] = &Measurement{
				Name:            name,
				Ts:              ts,
				Values:          values,
				Dimensions:      jsonDimensions,
				TypedDimensions: dimensions,
			}
		}
		return nil
	}
	c.submitters[bufferID] = submitter

	return func(values map[string]Val, dimensions map[string]interface{}) error {
		// Convert metrics to values
		for dim, val := range dimensions {
			metric, ok := val.(Val)
			if ok {
				delete(dimensions, dim)
				values[dim] = metric
			}
		}

		jsonDimensions, encodeErr := json.Marshal(dimensions)
		if encodeErr != nil {
			return errors.New("Unable to marshal dimensions: %v", encodeErr)
		}
		key := string(jsonDimensions)
		ts := time.Now()
		c.mx.Lock()
		err := submitter(key, ts, values, dimensions, jsonDimensions)
		c.mx.Unlock()
		return err
	}
}

// SetBatchInterval changes the batch interval
func (c *Client) SetBatchInterval(batchInterval time.Duration) {
	c.changeBatchInterval <- batchInterval
}

func (c *Client) sendPeriodically(batchInterval time.Duration) {
	log.Debugf("Reporting to Borda every %v", batchInterval)
	timer := time.NewTimer(batchInterval)
	resetTimer := func(alreadyRead bool, newInterval time.Duration) {
		if !alreadyRead && !timer.Stop() {
			<-timer.C
		}
		timer.Reset(newInterval)
	}

	lastFlush := time.Now()
	for {
		select {
		case <-timer.C:
			c.Flush()
			lastFlush = time.Now()
			resetTimer(true, batchInterval)
		case newBatchInterval := <-c.changeBatchInterval:
			if newBatchInterval == batchInterval {
				// ignore
				continue
			}
			batchInterval = newBatchInterval
			timeSinceLastFlush := time.Now().Sub(lastFlush)
			resetTimer(false, batchInterval-timeSinceLastFlush)
		}
	}
}

// Flush flushes any currently buffered data.
func (c *Client) Flush() {
	c.mx.Lock()
	currentBuffers := c.buffers
	// Clear out buffers
	c.buffers = make(map[int]map[string]*Measurement, len(c.buffers))
	c.mx.Unlock()

	// Count measurements
	numMeasurements := 0
	for _, buffer := range currentBuffers {
		numMeasurements += len(buffer)
	}
	if numMeasurements == 0 {
		log.Debug("Nothing to report")
		return
	}

	// Make batch
	batch := make(map[string][]*Measurement)
	for _, buffer := range currentBuffers {
		for _, m := range buffer {
			name := m.Name
			batch[name] = append(batch[name], m)
		}
	}

	log.Debugf("Attempting to report %d measurements to Borda", numMeasurements)
	numInserted, err := c.doSendBatch(batch)
	log.Debugf("Sent %d measurements", numInserted)
	if err != nil {
		log.Errorf("Error sending batch: %v", err)
	}
}

func (c *Client) doSendBatch(batch map[string][]*Measurement) (int, error) {
	for _, measurements := range batch {
		for _, m := range measurements {
			c.options.BeforeSubmit(m.Name, m.Ts, m.Values, m.Dimensions)
		}
	}
	return c.sender(batch)
}

func (c *Client) buildHTTPSender(hc *http.Client) func(batchByName map[string][]*Measurement) (int, error) {
	return func(batchByName map[string][]*Measurement) (int, error) {
		log.Debug("Sending batch with HTTP")

		numInserted := 0
		var batch []*Measurement
		for _, measurements := range batchByName {
			numInserted += len(measurements)
			batch = append(batch, measurements...)
		}
		buf := bufferPool.Get()
		defer bufferPool.Put(buf)
		err := json.NewEncoder(buf).Encode(batch)
		batchBytes := buf.Len()
		if err != nil {
			return 0, log.Errorf("Unable to encode measurements for reporting: %v", err)
		}

		req, decErr := http.NewRequest(http.MethodPost, c.options.url(), buf)
		if decErr != nil {
			return 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Borda-Local-Time", time.Now().UTC().Format(time.RFC3339Nano))

		resp, err := hc.Do(req)
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case 201:
			c.bytesSent += batchBytes
			log.Debugf("Sent %v to borda, cumulatively %v", humanize.Bytes(uint64(batchBytes)), humanize.Bytes(uint64(c.bytesSent)))
			return numInserted, nil
		case 400:
			errorMsg, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return 0, fmt.Errorf("Borda replied with 400, but error message couldn't be read: %v", err)
			}
			return 0, fmt.Errorf("Borda replied with the error: %v", string(errorMsg))
		default:
			return 0, fmt.Errorf("Borda replied with error %d", resp.StatusCode)
		}
	}
}

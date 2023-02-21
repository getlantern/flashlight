package config

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/anacrolix/missinggo/v2"
	"github.com/getlantern/dhtup"
)

type dhtFetcher struct {
	dhtupResource dhtup.Resource
}

func (d dhtFetcher) fetch() (retB []byte, sleep time.Duration, err error) {
	// There's some noise around default noSleep and default sleep times that I don't quite follow.
	// We can override this value for specific cases below should they warrant better handling. A
	// shorter timeout for transient network issues is a good default.
	retB, temporary, err := d.fetchTemporary()
	if temporary {
		sleep = 2 * time.Minute
	} else {
		sleep = noSleep
	}
	return
}

func (d dhtFetcher) fetchTemporary() (retB []byte, temporary bool, err error) {
	// The only reason this is here is it might vary on how big the payload is.
	ctx, cancel := context.WithTimeout(context.TODO(), 5*time.Minute)
	defer cancel()
	r, temporary, err := d.dhtupResource.Open(ctx)
	if err != nil {
		err = fmt.Errorf("opening dht resource: %w", err)
		return
	}
	defer r.Close()
	gzipReader, err := gzip.NewReader(
		missinggo.ContextedReader{R: r, Ctx: ctx})
	if err != nil {
		temporary = false
		err = fmt.Errorf("opening gzip: %w", err)
		return
	}
	retB, err = io.ReadAll(gzipReader)
	if err != nil {
		err = fmt.Errorf("reading all: %w", err)
	}
	// What if we timed out while reading?
	temporary = false
	return
}

package keepcurrent

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mholt/archiver/v3"
)

type webSource struct {
	url    string
	etag   string
	mx     sync.RWMutex
	client *http.Client
}

// FromWeb constructs a source from the given URL.
func FromWeb(url string) Source {
	return FromWebWithClient(url, http.DefaultClient)
}

// FromWebWithClient is the same as FromWeb but with a custom http.Client
func FromWebWithClient(url string, client *http.Client) Source {
	return &webSource{url: url, client: client}
}

// Fetch implements the Source interface
func (s *webSource) Fetch(ifNewerThan time.Time) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, s.url, nil)
	if err != nil {
		return nil, err
	}
	if !ifNewerThan.IsZero() {
		req.Header.Add("If-Modified-Since", ifNewerThan.Format(http.TimeFormat))
	}
	if s.getETag() != "" {
		req.Header.Add("If-None-Match", s.etag)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotModified {
		return nil, ErrUnmodified
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Unexpected HTTP status %v", resp.StatusCode)
	}
	etag := resp.Header.Get("ETag")
	if etag != "" {
		s.setETag(etag)
	}
	return resp.Body, nil
}

func (s *webSource) getETag() string {
	s.mx.RLock()
	defer s.mx.RUnlock()
	return s.etag
}

func (s *webSource) setETag(etag string) {
	s.mx.Lock()
	s.etag = etag
	s.mx.Unlock()
}

type tarGzSource struct {
	s            Source
	expectedName string
}

// FromTarGz wraps a source to decompress one specific file from the gzipped
// tarball.
func FromTarGz(s Source, expectedName string) Source {
	return &tarGzSource{s, expectedName}
}

func (s *tarGzSource) Fetch(ifNewerThan time.Time) (io.ReadCloser, error) {
	rc, err := s.s.Fetch(ifNewerThan)
	if err != nil {
		return nil, err
	}
	unzipper := archiver.NewTarGz()
	if err := unzipper.Open(rc, 0); err != nil {
		return nil, err
	}
	for {
		f, err := unzipper.Read()
		if err != nil {
			return nil, err
		}
		if f.Name() == s.expectedName {
			return chainedCloser{f, rc}, nil
		}
	}
}

type chainedCloser []io.ReadCloser

func (cc chainedCloser) Read(p []byte) (n int, err error) {
	return cc[0].Read(p)
}

func (cc chainedCloser) Close() error {
	var lastError error
	for _, c := range cc {
		if err := c.Close(); err != nil {
			lastError = err
		}
	}
	return lastError
}

type fileSource struct {
	path         string
	preprocessor func(io.ReadCloser) (io.ReadCloser, error)
}

// FromFile constructs a source from the given file path.
func FromFile(path string) Source {
	return &fileSource{path, nil}
}

// FromFileWithPreprocessor constructs a source from the given file path, while modifying the file data using preprocessor function
func FromFileWithPreprocessor(path string, preprocessor func(io.ReadCloser) (io.ReadCloser, error)) Source {
	return &fileSource{path, preprocessor}
}

func (s *fileSource) Fetch(ifNewerThan time.Time) (io.ReadCloser, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !ifNewerThan.IsZero() && ifNewerThan.Before(fi.ModTime()) {
		return nil, ErrUnmodified
	}
	var result io.ReadCloser
	result = f
	if s.preprocessor != nil {
		result, err = s.preprocessor(f)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

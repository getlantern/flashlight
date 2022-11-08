package keepcurrent

import (
	"io"
	"io/ioutil"
	"os"
)

type fileSink struct {
	path         string
	preprocessor func(io.Reader) (io.Reader, error)
}

// ToFile constructs a sink from the given file path. Writing to the file while
// reading from it (via FromFile) won't corrupt the file.
func ToFile(path string) Sink {
	return &fileSink{path, nil}
}

// ToFileWithPreprocessor constructs a sink from the given file path while modifying the data before writing to disk.
func ToFileWithPreprocessor(path string, preprocessor func(io.Reader) (io.Reader, error)) Sink {
	return &fileSink{path, preprocessor}
}

func (s *fileSink) UpdateFrom(r io.Reader) error {
	f, err := os.OpenFile(s.path, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return err
	}
	defer f.Close()
	if s.preprocessor != nil {
		r, err = s.preprocessor(r)
		if err != nil {
			return err
		}
	}
	_, err = io.Copy(f, r)
	return err
}

func (s *fileSink) String() string {
	return "file sink to " + s.path
}

type byteChannel struct {
	ch chan []byte
}

// ToChannel constructs a sink which sends all data to the given channel.
func ToChannel(ch chan []byte) Sink {
	return &byteChannel{ch}
}

func (s *byteChannel) UpdateFrom(r io.Reader) error {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	s.ch <- b
	return nil
}

func (s *byteChannel) String() string {
	return "byte channel"
}

package util

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	KB int64 = 1024
	MB int64 = 1024 * 1024
	GB int64 = 1024 * 1024 * 1024
)

// ZipOptions is a set of options for ZipFiles.
type ZipOptions struct {
	// The search pattern of the files / directories to be zipped, traversed
	// in lexical order.
	// The search pattern is described at the comments of path/filepath.Match.
	// As a special note, "**/*" doesn't match files not under a subdirectory.
	Glob string
	// The directory where the search pattern starts with, if specified. It
	// also serves as the root directory in zipped archive.
	Dir string
	// To replace Dir as the root directory in zipped archive, if specified.
	NewRoot string
	// The limit of total bytes of all the files in the archive.
	// All remaining files will be ignored if the limit would be hit.
	MaxBytes int64
}

var (
	sizeRegexp = regexp.MustCompile("^(\\d+)([k|m|g|K|M|G][b|B])?$")
	units      = map[string]int64{
		"":   1,
		"KB": KB,
		"MB": MB,
		"GB": GB,
	}
)

// ParseFileSize converts a string contains a positive integer and an optional
// KB/MB/GB unit to int64, or returns error.
func ParseFileSize(s string) (int64, error) {
	matched := sizeRegexp.FindStringSubmatch(s)
	if len(matched) == 0 {
		return 0, errors.New("malformed string")
	}
	i, _ := strconv.ParseInt(matched[1], 10, 64)
	return i * units[strings.ToUpper(matched[2])], nil
}

// ZipFile creates a zip archive per the options and writes to the writer.
func ZipFiles(writer io.Writer, opts ZipOptions) (err error) {
	glob := filepath.Join(opts.Dir, opts.Glob)
	matched, e := filepath.Glob(glob)
	if e != nil {
		return e
	}
	w := zip.NewWriter(writer)
	defer func() {
		if e := w.Close(); e != nil {
			err = e
		}
	}()

	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = math.MaxInt64
	}
	var totalBytes int64
	for _, source := range matched {
		nextTotal, e := zipFile(w, source, opts.Dir, opts.NewRoot, maxBytes, totalBytes)
		if e != nil || nextTotal > maxBytes {
			return e
		}
		totalBytes = nextTotal
	}
	return
}

func zipFile(w *zip.Writer, source string, baseDir string, newRoot string, limit int64, prevBytes int64) (newBytes int64, err error) {
	_, e := os.Stat(source)
	if e != nil {
		return prevBytes, fmt.Errorf("%s: stat: %v", source, e)
	}

	walkErr := filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking to %s: %v", fpath, err)
		}

		newBytes = prevBytes + info.Size()
		if newBytes > limit {
			return filepath.SkipDir
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("%s: getting header: %v", fpath, err)
		}

		if newRoot == "" {
			header.Name = fpath
		} else {
			header.Name = path.Join(newRoot, strings.TrimPrefix(fpath, baseDir))
		}
		if info.IsDir() {
			header.Name += "/"
			header.Method = zip.Store
		} else {
			header.Method = zip.Deflate
		}

		writer, err := w.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("%s: making header: %v", fpath, err)
		}

		if info.IsDir() {
			return nil
		}

		if !header.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(fpath)
		if err != nil {
			return fmt.Errorf("%s: opening: %v", fpath, err)
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		if err != nil && err != io.EOF {
			return fmt.Errorf("%s: copying contents: %v", fpath, err)
		}
		return nil
	})

	if walkErr != filepath.SkipDir {
		return newBytes, walkErr
	}
	return newBytes, nil
}

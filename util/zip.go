package util

import (
	"archive/zip"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	KB int64 = 1024
	MB int64 = 1024 * 1024
	GB int64 = 1024 * 1024 * 1024
)

// ZipOptions is a set of options for ZipFiles.
type ZipOptions struct {
	// The search pattern of the files / directories to be zipped, tranversed
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

	var totalBytes int64
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = math.MaxInt64
	}
	for _, source := range matched {
		if e := zipFile(w, source, opts.Dir, opts.NewRoot, maxBytes, &totalBytes); e != nil {
			return e
		}
		if totalBytes > maxBytes {
			return
		}
	}
	return
}

func zipFile(w *zip.Writer, source string, baseDir string, newRoot string, limit int64, size *int64) error {
	_, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%s: stat: %v", source, err)
	}

	err = filepath.Walk(source, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walking to %s: %v", fpath, err)
		}

		*size = *size + info.Size()
		if *size > limit {
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

		if header.Mode().IsRegular() {
			file, err := os.Open(fpath)
			if err != nil {
				return fmt.Errorf("%s: opening: %v", fpath, err)
			}
			defer file.Close()

			_, err = io.CopyN(writer, file, info.Size())
			if err != nil && err != io.EOF {
				return fmt.Errorf("%s: copying contents: %v", fpath, err)
			}
		}

		w.Flush()
		return nil
	})

	if err != filepath.SkipDir {
		return err
	}
	return nil
}

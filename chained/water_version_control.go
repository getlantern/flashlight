package chained

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type waterVersionControl struct {
	dir string
}

type wasmInfo struct {
	lastTimeLoaded time.Time
	path           string
}

func newWaterVersionControl(dir string) *waterVersionControl {
	return &waterVersionControl{
		dir: dir,
	}
}

// GetWASM returns the WASM file for the given transport
// Remember to Close the io.ReadCloser after using it
func (vc *waterVersionControl) GetWASM(ctx context.Context, transport string, downloader waterWASMDownloader) (io.ReadCloser, error) {
	path := filepath.Join(vc.dir, transport+".wasm")
	log.Debugf("trying to load file %q", path)
	f, err := os.Open(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, log.Errorf("failed to open file %s: %w", path, err)
	}

	if errors.Is(err, fs.ErrNotExist) || f == nil {
		if f != nil {
			f.Close()
		}
		response, err := vc.downloadWASM(ctx, transport, downloader)
		if err != nil {
			return nil, log.Errorf("failed to download WASM file: %w", err)
		}

		return response, nil
	}

	fi, err := f.Stat()
	if err != nil {
		return nil, log.Errorf("failed loading file info")
	}

	if fi.Size() == 0 {
		f.Close()
		log.Debug("loaded empty WASM file, downloading again")
		response, err := vc.downloadWASM(ctx, transport, downloader)
		if err != nil {
			return nil, log.Errorf("failed to download WASM file: %w", err)
		}
		return response, nil
	}

	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, log.Errorf("failed to seek file at the beginning: %w", err)
	}

	if err = vc.markUsed(transport); err != nil {
		return nil, log.Errorf("failed to update WASM history: %w", err)
	}
	log.Debugf("WASM file loaded, file size: %d", fi.Size())

	return f, nil
}

// Commit will update the history of the last time the WASM file was loaded
// and delete the outdated WASM files
func (vc *waterVersionControl) markUsed(transport string) error {
	f, err := os.Create(filepath.Join(vc.dir, transport+".last-loaded"))
	if err != nil {
		return log.Errorf("failed to create file %s: %w", transport+".last-loaded", err)
	}
	defer f.Close()

	if _, err = f.WriteString(strconv.FormatInt(time.Now().UTC().Unix(), 10)); err != nil {
		return log.Errorf("failed to write to file %s: %w", transport+".last-loaded", err)
	}
	if err = vc.cleanOutdated(); err != nil {
		return log.Errorf("failed to clean outdated WASMs: %w", err)
	}
	return nil
}

// unusedWASMsDeletedAfter is the time after which the WASM files are considered outdated
const unusedWASMsDeletedAfter = 7 * 24 * time.Hour

func (vc *waterVersionControl) cleanOutdated() error {
	wg := new(sync.WaitGroup)
	filesToBeDeleted := make([]string, 0)
	// walk through dir, load last-loaded and delete if older than unusedWASMsDeletedAfter
	err := filepath.Walk(vc.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return log.Errorf("failed to walk through dir: %w", err)
		}
		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".last-loaded" {
			return nil
		}

		lastLoaded, err := os.ReadFile(path)
		if err != nil {
			return log.Errorf("failed to read file %s: %w", path, err)
		}

		i, err := strconv.ParseInt(string(lastLoaded), 10, 64)
		if err != nil {
			return log.Errorf("failed to parse int: %w", err)
		}
		lastLoadedTime := time.Unix(i, 0)
		if time.Since(lastLoadedTime) > unusedWASMsDeletedAfter {
			filesToBeDeleted = append(filesToBeDeleted, path)
		}
		return nil
	})
	for _, path := range filesToBeDeleted {
		log.Debugf("deleting file: %q", path)
		wg.Add(1)
		go func() {
			defer wg.Done()
			transport := strings.TrimSuffix(filepath.Base(path), ".last-loaded")
			if err = os.Remove(filepath.Join(vc.dir, transport+".wasm")); err != nil {
				log.Errorf("failed to remove wasm file %s: %w", transport+".wasm", err)
				return
			}
			if err = os.Remove(path); err != nil {
				log.Errorf("failed to remove last-loaded file %s: %w", path, err)
				return
			}
		}()
	}
	wg.Wait()
	return err
}

func (vc *waterVersionControl) downloadWASM(ctx context.Context, transport string, downloader waterWASMDownloader) (io.ReadCloser, error) {
	outputPath := filepath.Join(vc.dir, transport+".wasm")
	log.Debugf("downloading WASM file and writing at %q", outputPath)
	f, err := os.Create(outputPath)
	if err != nil {
		return nil, log.Errorf("failed to create file %s: %w", transport, err)
	}

	if err = downloader.DownloadWASM(ctx, f); err != nil {
		return nil, log.Errorf("failed to download wasm: %w", err)
	}

	if _, err = f.Seek(0, 0); err != nil {
		return nil, log.Errorf("failed to seek to the beginning of the file: %w", err)
	}

	if err = vc.markUsed(transport); err != nil {
		return nil, log.Errorf("failed to update WASM history: %w", err)
	}

	return f, nil
}

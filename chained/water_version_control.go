package chained

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/flashlight/v7/proxied"
	"github.com/gocarina/gocsv"
)

type waterVersionControl struct {
	// contain all wasm files available locally
	// key is something like: protocol.version
	wasmFilesAvailable map[string]wasmInfo
	dir                string

	// history of the last time the WASM file was loaded
	history      []history
	historyMutex *sync.Mutex
}

type history struct {
	Transport      string    `csv:"transport"`
	LastTimeLoaded time.Time `csv:"last_time_loaded"`
}

type wasmInfo struct {
	version   string
	protocol  string
	builtWith string
	path      string
}

type VersionControl interface {
	GetWASM(ctx context.Context, transport string, urls []string) (io.ReadCloser, error)
	Commit(transport string) error
}

// NewVersionControl check if the received config is the same as we already
// have and if not, it'll try to fetch the newest WASM available.
func NewVersionControl(configDir string) (VersionControl, error) {
	wasmFilesAvailable, err := loadWASMFilesAvailable(configDir)
	if err != nil {
		return nil, log.Errorf("failed to load wasm files available: %v", err)
	}

	return &waterVersionControl{
		dir:                configDir,
		wasmFilesAvailable: wasmFilesAvailable,
		historyMutex:       &sync.Mutex{},
	}, nil
}

func loadWASMFilesAvailable(dir string) (map[string]wasmInfo, error) {
	// walk through the wasm directory and load all files available in the map
	files := make(map[string]wasmInfo)

	// check if the directory exists
	// if not, create it
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return nil, log.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	// walk through the directory and load all files available
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return log.Errorf("failed to walk through the directory %s: %v", dir, err)
		}

		if info.IsDir() {
			return nil
		}

		filename := info.Name()
		if filepath.Ext(filename) != ".wasm" {
			return nil
		}

		splitFilename := strings.Split(filename, ".")
		if len(splitFilename) < 3 {
			return log.Errorf("invalid filename: %s", filename)
		}

		files[filename] = wasmInfo{
			version:   splitFilename[0],
			protocol:  splitFilename[1],
			builtWith: splitFilename[2],
			path:      path,
		}
		return nil
	})

	if err != nil {
		return nil, log.Errorf("failed to walk through the directory %s: %v", dir, err)
	}

	return files, nil
}

// GetWASM returns the WASM file for the given transport
// Remember to Close the io.ReadCloser after using it
func (vc *waterVersionControl) GetWASM(ctx context.Context, transport string, urls []string) (io.ReadCloser, error) {
	info, ok := vc.wasmFilesAvailable[transport]
	if !ok {
		var err error
		info, err = vc.downloadWASM(ctx, transport, urls)
		if err != nil {
			return nil, log.Errorf("failed to download WASM file: %w", err)
		}

		vc.wasmFilesAvailable[transport] = info
	}

	f, err := os.Open(info.path)
	if err != nil {
		return nil, log.Errorf("failed to open file %s: %w", info.path, err)
	}

	return f, nil
}

// Commit will update the history of the last time the WASM file was loaded
// and delete the outdated WASM files
func (vc *waterVersionControl) Commit(transport string) error {
	vc.historyMutex.Lock()
	defer vc.historyMutex.Unlock()
	if err := vc.loadHistory(); err != nil {
		return log.Errorf("failed to load history: %w", err)
	}

	if err := vc.updateLastTimeLoaded(transport); err != nil {
		return log.Errorf("failed to update last time loaded: %w", err)
	}

	if err := vc.deleteOutdatedWASMFiles(); err != nil {
		return log.Errorf("failed to delete outdated WASM files: %w", err)
	}
	return nil
}

func (vc *waterVersionControl) updateLastTimeLoaded(transport string) error {
	for i, h := range vc.history {
		if h.Transport == transport {
			vc.history[i].LastTimeLoaded = time.Now()
			if err := vc.storeHistory(); err != nil {
				return log.Errorf("failed to store history: %w", err)
			}
			return nil
		}
	}

	vc.history = append(vc.history, history{
		Transport:      transport,
		LastTimeLoaded: time.Now(),
	})
	if err := vc.storeHistory(); err != nil {
		return log.Errorf("failed to store history: %w", err)
	}
	return nil
}

func (vc *waterVersionControl) loadHistory() error {
	historyCSV := filepath.Join(vc.dir, "history.csv")

	// if there's no history available, just initialize history
	if _, err := os.Stat(historyCSV); os.IsNotExist(err) {
		vc.history = []history{}
		return nil
	}

	f, err := os.Open(historyCSV)
	if err != nil {
		return log.Errorf("failed to open file %s: %w", historyCSV, err)
	}
	defer f.Close()

	history := []history{}
	if err := gocsv.UnmarshalFile(f, &history); err != nil {
		return log.Errorf("failed to unmarshal file %s: %w", historyCSV, err)
	}

	vc.history = history
	return nil
}

func (vc *waterVersionControl) deleteOutdatedWASMFiles() error {
	affectedTransports := make([]string, 0)
	for _, h := range vc.history {
		// check if the file is outdated
		sevenDaysAgo := time.Now().AddDate(0, 0, -7)
		if h.LastTimeLoaded.Before(sevenDaysAgo) {
			// delete the file
			info := vc.wasmFilesAvailable[h.Transport]
			if err := os.Remove(info.path); err != nil {
				return log.Errorf("failed to delete file %s: %w", h.Transport, err)
			}
			delete(vc.wasmFilesAvailable, h.Transport)
			affectedTransports = append(affectedTransports, h.Transport)
		}
	}

	// remove the affected transports from the history
	newHistory := make([]history, 0)
	for _, h := range vc.history {
		if !slices.Contains(affectedTransports, h.Transport) {
			newHistory = append(newHistory, h)
		}
	}
	vc.history = newHistory

	if err := vc.storeHistory(); err != nil {
		return log.Errorf("failed to store history: %w", err)
	}
	return nil
}

func (vc *waterVersionControl) storeHistory() error {
	historyCSV := filepath.Join(vc.dir, "history.csv")
	f, err := os.Create(historyCSV)
	if err != nil {
		return log.Errorf("failed to create file %s: %w", historyCSV, err)
	}
	defer f.Close()

	if err := gocsv.MarshalFile(&vc.history, f); err != nil {
		return log.Errorf("failed to marshal file %s: %w", historyCSV, err)
	}

	return nil
}

func (vc *waterVersionControl) downloadWASM(ctx context.Context, transport string, urls []string) (wasmInfo, error) {
	splitFilename := strings.Split(transport, ".")
	if len(splitFilename) < 3 {
		return wasmInfo{}, log.Errorf("invalid transport: %s", transport)
	}

	outputPath := filepath.Join(vc.dir, transport)
	f, err := os.Create(outputPath)
	if err != nil {
		return wasmInfo{}, log.Errorf("failed to create file %s: %w", transport, err)
	}
	defer f.Close()

	cli := httpClient
	if cli == nil {
		cli = proxied.ChainedThenDirectThenFrontedClient(1*time.Minute, "")
	}

	d, err := NewWASMDownloader(urls, cli)
	if err != nil {
		return wasmInfo{}, log.Errorf("failed to create wasm downloader: %w", err)
	}
	if err = d.DownloadWASM(ctx, f); err != nil {
		return wasmInfo{}, log.Errorf("failed to download wasm: %w", err)
	}

	return wasmInfo{
		version:   splitFilename[0],
		protocol:  splitFilename[1],
		builtWith: splitFilename[2],
		path:      outputPath,
	}, nil
}

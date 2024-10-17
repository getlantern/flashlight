package chained

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlantern/flashlight/v7/proxied"
)

type versionControl struct {
	// contain all wasm files available locally
	// key is something like: protocol.version
	wasmFilesAvailable map[string]wasmInfo
	dir                string
}

type wasmInfo struct {
	version   string
	protocol  string
	builtWith string
	path      string
}

type VersionControl interface {
	GetWASM(ctx context.Context, transport string, urls []string) (io.ReadCloser, error)
}

// NewVersionControl check if the received config is the same as we already
// have and if not, it'll try to fetch the newest WASM available.
func NewVersionControl(configDir string) (VersionControl, error) {
	wasmFilesAvailable, err := loadWASMFilesAvailable(configDir)
	if err != nil {
		return nil, log.Errorf("failed to load wasm files available: %v", err)
	}

	return &versionControl{
		dir:                configDir,
		wasmFilesAvailable: wasmFilesAvailable,
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

		// wasm filenames look like transport.version.wasm, we need to extract those vars
		// and create a map with the transport as key and the version as value

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
func (vc *versionControl) GetWASM(ctx context.Context, transport string, urls []string) (io.ReadCloser, error) {
	info, ok := vc.wasmFilesAvailable[transport]
	if !ok {
		outputPath := filepath.Join(vc.dir, transport)
		f, err := os.Create(outputPath)
		if err != nil {
			return nil, log.Errorf("failed to create file %s: %v", transport, err)
		}

		cli := httpClient
		if cli == nil {
			cli = proxied.ChainedThenDirectThenFrontedClient(1*time.Minute, "")
		}

		d, err := NewWASMDownloader(urls, cli)
		if err != nil {
			return nil, log.Errorf("failed to create wasm downloader: %s", err.Error())
		}
		if err = d.DownloadWASM(ctx, f); err != nil {
			return nil, log.Errorf("failed to download wasm: %s", err.Error())
		}
		f.Close()

		splitFilename := strings.Split(transport, ".")
		if len(splitFilename) < 3 {
			return nil, log.Errorf("invalid transport: %s", transport)
		}

		vc.wasmFilesAvailable[transport] = wasmInfo{
			version:   splitFilename[0],
			protocol:  splitFilename[1],
			builtWith: splitFilename[2],
			path:      outputPath,
		}

		info = vc.wasmFilesAvailable[transport]
	}

	f, err := os.Open(info.path)
	if err != nil {
		return nil, log.Errorf("failed to open file %s: %v", info.path, err)
	}

	return f, nil
}

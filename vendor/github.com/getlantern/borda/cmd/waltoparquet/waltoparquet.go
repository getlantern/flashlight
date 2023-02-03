// zenotool provides the ability to filter and merge zeno datafiles offline.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/bytemap"
	"github.com/getlantern/golog"
	"github.com/getlantern/hidden"
	"github.com/getlantern/wal"
	"github.com/getlantern/zenodb/encoding"

	"github.com/oxtoacart/bpool"
	"github.com/vharitonsky/iniflags"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
)

const (
	baseFilename = "walexport.parquet"
)

var (
	log = golog.LoggerFor("waltoparquet")

	outDir  = flag.String("outdir", "walexport", "Name of directory to which to write output files")
	maxSize = flag.Int("limit", 20000000000, "Maximum bytes to read from WAL per output file, defaults to 20GB. Beyond this, new files are written with suffixes .1, .2, etc.")

	pool = bpool.NewBytePool(1, 65536)

	sanitizeIPRegex = regexp.MustCompile(`(\[?([0-9A-Fa-f]{1,4}:){7}[0-9A-Fa-f]{1,4}|(\d{1,3}\.){3}\d{1,3})(\]?:[0-9]+)?`)

	// Assume 0 high water mark
	hwm = make(wal.Offset, wal.OffsetSize)
)

func main() {
	iniflags.SetAllowUnknownFlags(true)
	iniflags.Parse()

	args := flag.Args()
	if len(args) != 1 {
		log.Fatal("Please specify a single wal directory")
	}

	if *outDir == "" {
		log.Fatal("Please specify an outdir")
	}

	if err := os.MkdirAll(*outDir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		log.Fatalf("Unable to create outdir %v: %v", *outDir, err)
	}
	if files, err := ioutil.ReadDir(*outDir); err != nil {
		log.Fatalf("Unable to list existing files in %v: %v", *outDir, err)
	} else {
		filenames := make([]string, 0, len(files))
		for _, file := range files {
			if strings.HasSuffix(file.Name(), baseFilename) {
				filenames = append(filenames, file.Name())
			}
		}
		sort.Strings(filenames)
		if len(filenames) > 0 {
			latestFile := filenames[len(filenames)-1]
			mainParts := strings.Split(latestFile, "_")
			parts := strings.Split(mainParts[0], ".")
			fileSequence, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				log.Fatalf("Unable to parse integer from %v", parts[0])
			}
			position, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				log.Fatalf("Unable to parse integer from %v", parts[1])
			}
			hwm = wal.NewOffset(fileSequence, position)
			log.Debugf("Will begin exporting rows as of %v", hwm.TS())
		}
	}

	w, err := wal.Open(&wal.Opts{
		Dir: args[0],
	})
	if err != nil {
		log.Fatalf("Unable to open wal: %v", err)
	}
	defer w.Close()

	r, err := w.NewReader("reader", nil, pool.Get)
	if err != nil {
		log.Fatalf("Unable to open reader: %v", err)
	}
	defer r.Close()

	for {
		if !processFile(r) {
			break
		}
	}

	log.Debug("Finished!")
}

func processFile(r *wal.Reader) (more bool) {
	tempFilename := filepath.Join(*outDir, fmt.Sprintf("%v_", baseFilename))
	out, err := local.NewLocalFileWriter(tempFilename)
	if err != nil {
		log.Fatalf("Unable to open local outfile: %v", err)
	}
	defer out.Close()

	pw, err := writer.NewParquetWriter(out, &Row{}, int64(runtime.NumCPU()))
	if err != nil {
		log.Fatalf("Unable to open parquet writer: %v", err)
	}
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	bytesRead := 0
readloop:
	for {
		stop := make(chan interface{})
		go func() {
			select {
			case <-stop:
				return
			case <-time.After(1 * time.Second):
				log.Debug("Closing wal due to timeout")
				r.Close()
			}
		}()
		b, err := r.Read()
		if err != nil {
			log.Debugf("Stopped reading because of %v", err)
			break
		}
		close(stop)

		if r.Offset().After(hwm) {
			processRow(pw, b)
			bytesRead += len(b)
		}
		pool.Put(b)

		if bytesRead >= *maxSize {
			more = true
			break readloop
		}
	}

	err = pw.WriteStop()
	if err != nil {
		log.Fatalf("Unable to stop writing to parquet file: %v", err)
	}

	// commit file and update high watermark
	if err := out.Close(); err != nil {
		log.Fatalf("Unable to close output file %v: %v", tempFilename, err)
	}
	filename := filepath.Join(*outDir, fmt.Sprintf("%d.%d_%v", r.Offset().FileSequence(), r.Offset().Position(), baseFilename))
	if err := os.Rename(tempFilename, filename); err != nil {
		log.Fatalf("Unable to rename temp file %v to %v: %v", tempFilename, filename, err)
	}

	return
}

func processRow(pw *writer.ParquetWriter, b []byte) {
	defer func() {
		p := recover()
		if p != nil {
			log.Debugf("Ignoring panic: %v", p)
		}
	}()

	row := buildRow(b)
	err := pw.Write(row)
	if err != nil {
		log.Fatalf("Error writing to parquet file: %v", err)
	}
}

func buildRow(data []byte) *Row {
	tsd, remain := encoding.Read(data, encoding.Width64bits)
	ts := encoding.TimeFromBytes(tsd)
	dimsLen, remain := encoding.ReadInt32(remain)
	dims, remain := encoding.Read(remain, dimsLen)
	valsLen, remain := encoding.ReadInt32(remain)
	vals, _ := encoding.Read(remain, valsLen)
	// Split the dims and vals so that holding on to one doesn't force holding on
	// to the other. Also, we need copies for both because the WAL read buffer
	// will change on next call to wal.Read().
	dimsBM := make(bytemap.ByteMap, len(dims))
	valsBM := make(bytemap.ByteMap, len(vals))
	copy(dimsBM, dims)
	copy(valsBM, vals)

	return &Row{
		SuccessCount:    int64(getInt64(valsBM, "success_count")),
		ErrorCount:      int64(getInt64(valsBM, "error_count")),
		NanosSinceEpoch: ts.UnixNano(),
		Op:              getString(dimsBM, "op"),
		ProxyName:       getString(dimsBM, "proxy_name"),
		GeoCountry:      getString(dimsBM, "geo_country"),
		DC:              getString(dimsBM, "dc"),
		DeviceID:        getString(dimsBM, "device_id"),
		OsName:          getString(dimsBM, "os_name"),
		App:             getString(dimsBM, "app"),
		AppVersion:      getString(dimsBM, "app_version"),
		IsPro:           dimsBM.Get("is_pro") == true,
		Error:           sanitizeIP(hidden.Clean(getString(dimsBM, "error"))),
		ErrorText:       sanitizeIP(hidden.Clean(getString(dimsBM, "error_text"))),
	}
}

func getInt64(bm bytemap.ByteMap, key string) int64 {
	switch t := bm.Get(key).(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case float32:
		return int64(t)
	case float64:
		return int64(t)
	default:
		return 0
	}
}

func getString(bm bytemap.ByteMap, key string) string {
	val, _ := bm.Get(key).(string)
	return val
}

func sanitizeIP(str string) string {
	return sanitizeIPRegex.ReplaceAllString(str, "<addr>")
}

type Row struct {
	SuccessCount    int64  `parquet:"name=success_count, type=INT64"`
	ErrorCount      int64  `parquet:"name=error_count, type=INT64"`
	NanosSinceEpoch int64  `parquet:"name=nanos_since_epoch, type=INT64"`
	Op              string `parquet:"name=op, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	ProxyName       string `parquet:"name=proxy_name, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	GeoCountry      string `parquet:"name=geo_country, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	DC              string `parquet:"name=dc, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	DeviceID        string `parquet:"name=device_id, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	OsName          string `parquet:"name=os_name, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	App             string `parquet:"name=app, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	AppVersion      string `parquet:"name=app_version, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	IsPro           bool   `parquet:"name=is_pro, type=BOOLEAN"`
	Error           string `parquet:"name=error, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
	ErrorText       string `parquet:"name=error_text, type=BYTE_ARRAY, convertedtype=UTF8, encoding=PLAIN_DICTIONARY"`
}

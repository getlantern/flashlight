package util

import (
	"archive/zip"
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZipFiles(t *testing.T) {
	var buf bytes.Buffer
	err := ZipFiles(&buf, ZipOptions{Glob: "test_zip_src/*.txt*"})
	if !assert.NoError(t, err) {
		return
	}
	expectedFiles := []string{
		"test_zip_src/hello.txt",
		"test_zip_src/hello.txt.1",
		"test_zip_src/large.txt",
		"test_zip_src/zzzz.txt.2",
	}
	testZipFiles(t, buf.Bytes(), expectedFiles)
}

func TestZipFilesWithDir(t *testing.T) {
	var buf bytes.Buffer
	err := ZipFiles(&buf, ZipOptions{Glob: "*.txt*", Dir: "test_zip_src"})
	if !assert.NoError(t, err) {
		return
	}
	expectedFiles := []string{
		"test_zip_src/hello.txt",
		"test_zip_src/hello.txt.1",
		"test_zip_src/large.txt",
		"test_zip_src/zzzz.txt.2",
	}
	testZipFiles(t, buf.Bytes(), expectedFiles)
}

func TestZipFilesWithMaxBytes(t *testing.T) {
	var buf bytes.Buffer
	err := ZipFiles(&buf, ZipOptions{Glob: "*.txt*", Dir: "test_zip_src", MaxBytes: 1 * KB})
	if !assert.NoError(t, err) {
		return
	}
	expectedFiles := []string{
		"test_zip_src/hello.txt",
		"test_zip_src/hello.txt.1",
	}
	testZipFiles(t, buf.Bytes(), expectedFiles)
}

func TestZipFilesWithNewRoot(t *testing.T) {
	var buf bytes.Buffer
	err := ZipFiles(&buf, ZipOptions{Glob: "*.txt*", Dir: "test_zip_src", NewRoot: "new_root"})
	if !assert.NoError(t, err) {
		return
	}
	expectedFiles := []string{
		"new_root/hello.txt",
		"new_root/hello.txt.1",
		"new_root/large.txt",
		"new_root/zzzz.txt.2",
	}
	testZipFiles(t, buf.Bytes(), expectedFiles)
}

func testZipFiles(t *testing.T, zipped []byte, expectedFiles []string) {
	reader, eread := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if !assert.NoError(t, eread) {
		return
	}
	if !assert.Equal(t, len(expectedFiles), len(reader.File), "should not include extra files and files that would exceed MaxBytes") {
		return
	}
	for idx, file := range reader.File {
		t.Log(file.Name)
		assert.Equal(t, expectedFiles[idx], file.Name)
		if !strings.Contains(file.Name, "hello.txt") {
			continue
		}
		fileReader, err := file.Open()
		if !assert.NoError(t, err) {
			return
		}
		defer fileReader.Close()
		actual, _ := ioutil.ReadAll(fileReader)
		assert.Equal(t, []byte("world\n"), actual)
	}
}

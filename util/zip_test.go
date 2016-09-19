package util

import (
	"archive/zip"
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZipFile(t *testing.T) {
	var buf bytes.Buffer
	err := ZipFile(&buf, "test_zip_src/")
	if !assert.NoError(t, err) {
		return
	}
	zipped := buf.Bytes()
	reader, eread := zip.NewReader(bytes.NewReader(zipped), int64(len(zipped)))
	if !assert.NoError(t, eread) {
		return
	}
	found := false
	for _, file := range reader.File {
		t.Log(file.Name)
		if file.Name == "test_zip_src/hello.txt" {
			found = true
			fileReader, err := file.Open()
			if !assert.NoError(t, err) {
				return
			}
			defer fileReader.Close()
			actual, _ := ioutil.ReadAll(fileReader)
			assert.Equal(t, []byte("world\n"), actual)
		}
	}
	assert.True(t, found)
}

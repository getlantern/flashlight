package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func testSanitize(t *testing.T, input string, expectedOutput string) {
	sanitized := SanitizePathString(input)
	assert.Equal(t, expectedOutput, sanitized)
}

func testTrim(t *testing.T, numChars uint, input string, trimFront bool, expectedOutput string) {
	trimmed := TrimStringAsRunes(numChars, input, trimFront)
	assert.Equal(t, expectedOutput, trimmed)
}

func TestSanitizeAllBad(t *testing.T) {
	testSanitize(t, input, output)
}

func TestSanitizeMixed(t *testing.T) {
	input := "/../th...is#*s!@#--$.&__%^&*(8)\n|][:11;]stringisbad..$$%%~~"
	ouput := "-th-is-s----.-__-8-11-stringisbad-"
	testSanitize(t, input, output)
}

func TestSanitizeAllGood(t *testing.T) {
	testSanitize(t, input, output)
}

func TestSanitizeEmptyString(t *testing.T) {
	testSanitize(t, input, output)
}

func TestSanitizeChinese(t *testing.T) {
	testSanitize(t, input, output)
}

func TestSanitizeFarsi(t *testing.T) {
	testSanitize(t, input, output)
}

func TestTrimEmptyString(t *testing.T) {
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimUTF8(t *testing.T) {
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimASCII(t *testing.T) {
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimEntireString(t *testing.T) {
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimOutOfBounds(t *testing.T) {
	testTrim(t, numChars, input, trimFront, expectedOutput)

}

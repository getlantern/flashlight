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
	input := "##########"
	output := "-"
	testSanitize(t, input, output)
}

func TestSanitizeMixed(t *testing.T) {
	input := "/../th...is#*s!@#--$.&__%^&*(8)\n|][:11;]stringisbad..$$%%~~"
	output := "-th-is-s----.-__-8-11-stringisbad-"
	testSanitize(t, input, output)
}

func TestSanitizeAllGood(t *testing.T) {
	input := "hello_world.png"
	testSanitize(t, input, input)
}

func TestSanitizeEmptyString(t *testing.T) {
	input := ""
	output := ""
	testSanitize(t, input, output)
}

func TestSanitizeChinese(t *testing.T) {
	input := "中國哲學書電子化計劃"
	testSanitize(t, input, input)
}

func TestSanitizeFarsi(t *testing.T) {
	input := "الفبایخنداشترحروف"
	testSanitize(t, input, input)
}

func TestTrimEmptyString(t *testing.T) {
	input := ""
	var numChars uint
	numChars = 5
	trimFront := true
	expectedOutput := ""
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimNothing(t *testing.T) {
	input := "this is a string"
	var numChars uint
	numChars = 17
	trimFront := true
	testTrim(t, numChars, input, trimFront, input)
}

func TestTrimWhitespace(t *testing.T) {
	input := "       "
	var numChars uint
	numChars = 3
	trimFront := true
	expectedOutput := "   "
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

func TestTrimToEmpty(t *testing.T) {
	input := "hello world"
	var numChars uint
	numChars = 0
	trimFront := true
	expectedOutput := ""
	testTrim(t, numChars, input, trimFront, expectedOutput)
}

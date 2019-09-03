package util

import (
	"regexp"
)

var (
	// use non-capture group to remove from our string all characters that are either
	// 1: two or more periods in a row
	// __OR__
	// 2: aren't numbers \pN && aren't common letters \pL && aren't hyphen && aren't underscore && aren't period && aren't Chinese characters \p{Han} && aren't Farsi/Other characters
	blacklist = regexp.MustCompile(`(?:\.{2,}|[^\pN\pL\-\_\.\p{Han}\p{Inherited}])+`)
)

func SanitizePathString(s string) (output string) {
	output = blacklist.ReplaceAllLiteralString(s, "-")
	return
}

// Trim string to numChars: we only care about the number of runes (encoded characters of variable bytes) so we don't cut up multibyte characters
func TrimStringAsRunes(numChars uint, s string, trimFront bool) string {
	runes := []rune(s)

	if uint(len(runes)) > numChars {
		if trimFront {
			s = string(runes[uint(len(runes))-numChars:])
		} else {
			s = string(runes[:numChars])
		}
	}
	return s
}

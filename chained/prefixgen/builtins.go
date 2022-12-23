package prefixgen

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

const defaultRandomStringSet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// builtin represents a built-in function for use in prefix generators. A builtin parses and closes
// over a slice of string arguments. Non-deterministic builtins will be re-evaluated each time,
// producing new results.
type builtin func([]string) (func() []byte, error)

// Builtins is a collection of built-in functions which can be referenced in the generator provided
// to New.
var Builtins = map[string]builtin{
	"hex":           Hex,
	"random_string": RandomString,
	"random_int":    RandomInt,
	"random_bytes":  RandomBytes,
}

var mathrand = rand.New(rand.NewSource(time.Now().UnixNano()))

// Hex is a builtin function (see New and Builtins).
//
// Args:
//
//	0 (hex input): string
//		even in length, containing only characters in [0-9A-Fa-f]
//
// Parses argument 0 into raw bytes and prints to the output.
//
// Example:
//
//	$hex(BEEFFACE33)
func Hex(args []string) (func() []byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 arg, received %d", len(args))
	}
	b, err := hex.DecodeString(args[0])
	if err != nil {
		return nil, err
	}
	return func() []byte { return b }, nil
}

// RandomString is a builtin function (see New and Builtins).
//
// Args:
//
//	0 (min length): integer
//	1 (max length): integer
//	2 (character set): string, optional
//
// Produces a string, whose length will be [min length, max length). If a character set is provided,
// only the specified characters will be considered. The default character set is the set of 26
// alphabetic characters, lower and upper case.
//
// Examples:
//
//	$random_string(1, 10)
//	$random_string(1, 10, aeiou)
func RandomString(args []string) (func() []byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expected at least 2 args, received %d", len(args))
	}

	minLen, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse min len: %w", err)
	}
	maxLen, err := strconv.Atoi(args[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse max len: %w", err)
	}

	chars := []rune(defaultRandomStringSet)
	if len(args) > 2 {
		chars = []rune(args[2])
	}

	return func() []byte {
		runes := make([]rune, mathrand.Intn(maxLen-minLen)+minLen)
		for i := range runes {
			runes[i] = chars[mathrand.Intn(len(chars))]
		}
		return []byte(string(runes))
	}, nil
}

// RandomInt is a builtin function (see New and Builtins).
//
// Args:
//
//	0 (min): integer
//	1 (max): integer
//
// Produces an integer in the range [min, max).
//
// Examples:
//
//	$random_int(0, 10)
//	$random_int(-1000, 1000)
func RandomInt(args []string) (func() []byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 args, received %d", len(args))
	}

	minLen, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse min len: %w", err)
	}
	maxLen, err := strconv.Atoi(args[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse max len: %w", err)
	}

	return func() []byte {
		n := mathrand.Intn(maxLen-minLen) + minLen
		return []byte(strconv.Itoa(n))
	}, nil

}

// RandomBytes is a builtin function (see New and Builtins).
//
// Args:
//
//	0 (min length): integer
//	1 (max length): integer
//	2 (byte set): string, optional
//		even in length, containing only characters in [0-9A-Fa-f]
//		each group of two characters represents a possible choice
//
// Produces a byte string, whose length will be [min length, max length). If a byte set is provided,
// only the specified bytes will be considered. The default byte set is all possible bytes.
//
// Examples:
//
//	$random_bytes(1, 10)
//	$random_bytes(1, 10, 010203A1A2A3)
func RandomBytes(args []string) (func() []byte, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("expected at least 2 args, received %d", len(args))
	}

	minLen, err := strconv.Atoi(args[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse min len: %w", err)
	}
	maxLen, err := strconv.Atoi(args[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse max len: %w", err)
	}

	var possibilities []byte
	if len(args) > 2 {
		possibilities, err = hex.DecodeString(args[2])
		if err != nil {
			return nil, fmt.Errorf("failed to decode byte set")
		}
	} else {
		possibilities = make([]byte, 256)
		for i := 0; i < 256; i++ {
			possibilities[i] = byte(i)
		}
	}

	return func() []byte {
		b := make([]byte, mathrand.Intn(maxLen-minLen)+minLen)
		for i := range b {
			b[i] = possibilities[mathrand.Intn(len(possibilities))]
		}
		return b
	}, nil
}

package prefixgen

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// currentMajorVersion defines the current major version of the custom prefix specification.
const currentMajorVersion = 1

var (
	generatorRegexp    = regexp.MustCompile(`^v([0-9]+)\.([0-9]+) ([\s\S]*)$`)
	functionCallRegexp = regexp.MustCompile(`\$([a-zA-Z][a-zA-Z_]*)\(([^\)]*)\)`)
)

// parse parses a generator as defined by New and returns a Go function implementing the generator.
func parse(generator string, builtins map[string]builtin) (PrefixGen, error) {
	matches := generatorRegexp.FindStringSubmatch(generator)
	if len(matches) != 4 {
		return nil, errors.New("malformed version specifier")
	}

	// n.b. We can ignore the error because of the regular expression.
	majorVersion, _ := strconv.Atoi(matches[1])
	if majorVersion > currentMajorVersion {
		return nil, fmt.Errorf(
			"major version %d exceeds supported version %d", majorVersion, currentMajorVersion)
	}

	// Parse out all function calls.

	generator = matches[3]
	rawFunctionCalls := functionCallRegexp.FindAllString(generator, -1)

	// Collect a slice of unevaluated function calls (parse the functions and their args, but do not
	// yet evaluate).

	functionCalls := make([]func() []byte, len(rawFunctionCalls))
	for i, call := range rawFunctionCalls {
		submatches := functionCallRegexp.FindStringSubmatch(call)
		if len(submatches) != 3 {
			return nil, errors.New("malformed function call")
		}

		name, rawArgs := submatches[1], submatches[2]
		parseBuiltin, ok := builtins[name]
		if !ok {
			return nil, fmt.Errorf("unsupported builtin '%s'", name)
		}
		if strings.Contains(rawArgs, "$") {
			return nil, fmt.Errorf("nested functions are not supported")
		}

		args := strings.Split(rawArgs, ",")
		if len(args) == 1 && args[0] == "" {
			args = []string{}
		}
		for i, arg := range args {
			args[i] = strings.TrimSpace(arg)
		}
		eval, err := parseBuiltin(args)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", name, err)
		}

		functionCalls[i] = eval
	}

	// When we are called to generate the prefix, evaluate all function calls. This ensures that
	// non-deterministic functions are re-evaluated each time.
	return func() []byte {
		callIndex := 0
		replaceFunc := func(_ []byte) []byte {
			replace := functionCalls[callIndex]()
			callIndex++
			return replace
		}

		return functionCallRegexp.ReplaceAllFunc([]byte(generator), replaceFunc)
	}, nil
}

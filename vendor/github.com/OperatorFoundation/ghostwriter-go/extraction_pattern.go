package ghostwriter

import (
	"errors"
	"regexp"
	"strconv"
)

type ExtractionPattern struct {
	Expression string
	Type       Types
}

func (pattern ExtractionPattern) extract(extractString string) (*string, error) {
	return pattern.extractor(extractString)
}

func (pattern ExtractionPattern) convert(convertString string) (Detail, error) {
	switch pattern.Type {
	case String:
		return &DetailString{convertString}, nil
	case Int:
		convertInt, convertError := strconv.Atoi(convertString)
		if convertError != nil {
			return nil, convertError
		}

		return &DetailInt{convertInt}, nil
	case UInt:
		convertUInt, convertError := strconv.ParseUint(convertString, 10, 64)
		if convertError != nil {
			return nil, convertError
		}
		return &DetailUInt{uint(convertUInt)}, nil
	case Float:
		convertFloat, convertError := strconv.ParseFloat(convertString, 32)
		if convertError != nil {
			return nil, convertError
		}
		return &DetailFloat{float32(convertFloat)}, nil
	case Data:
		return &DetailData{[]byte(convertString)}, nil
	}
	return nil, errors.New("convert(): unknown pattern type")
}

func (pattern ExtractionPattern) extractor(x string) (*string, error) {
	regex, regexError := regexp.Compile(pattern.Expression)
	if regexError != nil {
		return nil, regexError
	}

	matches := regex.FindStringSubmatch(x)
	if len(matches) == 0 {
		return nil, errors.New("extractor(): could not find match")
	}
	return &matches[1], nil
}

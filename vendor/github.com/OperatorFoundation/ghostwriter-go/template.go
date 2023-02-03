package ghostwriter

import (
	"errors"
	"strconv"
	"strings"
)

type Template struct {
	String string
}

func (temp *Template) apply(index int, detail Detail) (*Template, error) {
	oldText := "$" + strconv.Itoa(index)
	newText := detail.ToString()

	if !strings.Contains(temp.String, oldText) {
		return nil, errors.New("template.apply(): no substitution: " + oldText)
	}

	result := strings.ReplaceAll(temp.String, oldText, newText)

	return &Template{result}, nil
}

func (temp *Template) extract(index int, pattern ExtractionPattern, source string) (*Template, *string, *Detail, error) {
	oldText := "$" + strconv.Itoa(index)
	extractIndex := strings.Index(temp.String, oldText)
	if extractIndex == -1 {
		return nil, nil, nil, errors.New("template.extract(): failed to find index")
	}

	templatePreludeIndex := extractIndex + len(oldText) // exclusive
	sourcePreludeIndex := extractIndex
	prelude := temp.String[:extractIndex] // exclusive
	if !strings.HasPrefix(source, prelude) {
		return nil, nil, nil, errors.New("template.extract(): source and prelude from template do not match")
	}

	templateRest := temp.String[templatePreludeIndex:]
	sourceRest := source[sourcePreludeIndex:]

	result, extractError := pattern.extract(sourceRest)
	if extractError != nil {
		return nil, nil, nil, extractError
	}

	convertedDetail, detailError := pattern.convert(*result)
	if detailError != nil {
		return nil, nil, nil, detailError
	}

	matchIndex := strings.Index(sourceRest, *result)
	if matchIndex == -1 {
		return nil, nil, nil, errors.New("template.extract(): source does not match template")
	}

	endMatchIndex := matchIndex + len(*result)
	resultEnd := sourceRest[endMatchIndex:]

	return &Template{templateRest}, &resultEnd, &convertedDetail, nil
}

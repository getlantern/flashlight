package ghostwriter

import "errors"

func Generate(template *Template, details []Detail) (*string, error) {
	var result = template
	var applyError error

	if len(details) >= 9 {
		return nil, errors.New("ghostwriter.Generate(): we do not support more than 9 details")
	}

	for index, detail := range details {
		result, applyError = result.apply(index+1, detail)
		if applyError != nil {
			return nil, applyError
		}
	}

	return &result.String, nil
}

func Parse(template *Template, patterns []ExtractionPattern, parseString string) ([]Detail, error) {
	var working = template
	var source = parseString
	var details = make([]Detail, 0)

	if len(patterns) >= 9 {
		return nil, errors.New("ghostwriter.Parse(): we do not support more than 9 patterns")
	}

	for index, pattern := range patterns {
		newTemplate, newSource, detail, extractError := working.extract(index+1, pattern, source)
		if extractError != nil {
			return nil, extractError
		}

		working = newTemplate
		source = *newSource
		details = append(details, *detail)
	}

	if working.String != source {
		return nil, errors.New("ghostwriter.Parse() final working string and source do not match")
	}

	return details, nil
}

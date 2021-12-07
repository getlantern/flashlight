package status

import (
	"bytes"
	_ "embed"
	"errors"
	"html/template"
	"strings"
)

//go:embed generic_error.html
var generic_error_html string

type errorAccesingPageT struct {
	ServerName   string
	ErrorMessage string
}

func normalizeError(err error) string {
	if err != nil {
		content := strings.SplitN(strings.TrimSpace(err.Error()), "\n", 2)
		return strings.TrimSpace(content[0])
	}
	return ""
}

// ErrorAccessingPage creates and returns a generic "error accessing page" error.
func ErrorAccessingPage(server string, errMessage error) ([]byte, error) {
	var err error
	var tmpl *template.Template

	if errMessage == nil {
		errMessage = errors.New("Unknown error.")
	}

	tmpl, err = template.New("status_error").Parse(generic_error_html)
	if err != nil {
		return nil, err
	}

	data := errorAccesingPageT{
		ServerName:   server,
		ErrorMessage: normalizeError(errMessage),
	}

	out := bytes.NewBuffer(nil)

	if err = tmpl.Execute(out, data); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

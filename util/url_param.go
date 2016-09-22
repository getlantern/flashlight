package util

import (
	"net/url"
)

func SetURLParam(in string, key string, value string) string {
	out, err := url.Parse(in)
	if err != nil {
		return in
	}
	values, err := url.ParseQuery(out.RawQuery)
	if err != nil {
		return in
	}
	values.Set(key, value)
	out.RawQuery = values.Encode()
	return out.String()
}

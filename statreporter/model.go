package statreporter

import (
	"fmt"
)

const (
	increments = "increments"
	gauges     = "gauges"
)

const (
	set = iota
	add = iota
)

type Dims struct {
	keys   []string
	values []string
}

type update struct {
	dims     *Dims
	category string
	action   int
	key      string
	val      int64
}

type UpdateBuilder struct {
	dims     *Dims
	category string
	key      string
}

func Dim(key string, value string) *Dims {
	return &Dims{
		[]string{key},
		[]string{value},
	}
}

func (dims *Dims) And(key string, value string) *Dims {
	return &Dims{
		append(dims.keys, key),
		append(dims.values, value),
	}
}

func (dims *Dims) String() string {
	s := ""
	for i, key := range dims.keys {
		sep := ","
		if i == 0 {
			sep = ""
		}
		s = fmt.Sprintf("%s%s%s=%s", s, sep, key, dims.values[i])
	}
	return s
}

func (dims *Dims) Increment(key string) *UpdateBuilder {
	return &UpdateBuilder{
		dims,
		increments,
		key,
	}
}

func (dims *Dims) Gauge(key string) *UpdateBuilder {
	return &UpdateBuilder{
		dims,
		gauges,
		key,
	}
}

func (b *UpdateBuilder) Add(val int64) {
	postUpdate(&update{
		b.dims,
		b.category,
		add,
		b.key,
		val,
	})
}

func (b *UpdateBuilder) Set(val int64) {
	postUpdate(&update{
		b.dims,
		b.category,
		set,
		b.key,
		val,
	})
}

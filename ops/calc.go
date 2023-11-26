package ops

import (
	"encoding/json"
)

// Val represents a value that can be reduced by a reducing submitter.
type Val interface {
	// Get gets the final value (either a float64 or []float64)
	Get() interface{}

	// Merges merges another value with this one, using logic specific to the type
	// of value.
	Merge(b Val) Val
}

// Sum is float value that gets reduced by plain addition.
type Sum float64

func (a Sum) Merge(b Val) Val {
	if b == nil {
		return a
	}
	i := a.Get().(float64)
	ii := b.Get().(float64)
	return Sum(i + ii)
}

func (a Sum) Get() interface{} {
	return float64(a)
}

// Float is an alias for Sum, it is deprecated, using Sum instead
type Float Sum

func (a Float) Merge(b Val) Val {
	if b == nil {
		return a
	}
	i := a.Get().(float64)
	ii := b.Get().(float64)
	return Sum(i + ii)
}

func (a Float) Get() interface{} {
	return float64(a)
}

// Min is a float value that gets reduced by taking the lowest value.
type Min float64

func (a Min) Merge(b Val) Val {
	if b == nil {
		return a
	}
	i := a.Get().(float64)
	ii := b.Get().(float64)
	if i < ii {
		return a
	}
	return b
}

func (a Min) Get() interface{} {
	return float64(a)
}

// Max is a float value that gets reduced by taking the highest value.
type Max float64

func (a Max) Merge(b Val) Val {
	if b == nil {
		return a
	}
	i := a.Get().(float64)
	ii := b.Get().(float64)
	if i > ii {
		return a
	}
	return b
}

func (a Max) Get() interface{} {
	return float64(a)
}

// Avg creates a value that gets reduced by taking the arithmetic mean of the
// values.
func Avg(val float64) Val {
	return Average{val, 1}
}

// WeightedAvg is like Avg but the value is weighted by a given weight.
func WeightedAvg(val float64, weight float64) Val {
	return Average{val * weight, weight}
}

// Average holds the total plus a count in order to calculate the arithmatic mean
type Average [2]float64

func (a Average) Merge(_b Val) Val {
	if _b == nil {
		return a
	}
	switch b := _b.(type) {
	case Average:
		return Average{a[0] + b[0], a[1] + b[1]}
	default:
		return Average{a[0] + b.Get().(float64), a[1] + 1}
	}
}

func (a Average) Get() interface{} {
	if a[1] == 0 {
		return float64(0)
	}
	return float64(a[0] / a[1])
}

// Average is marshalled to JSON as its final float64 value.
func (a Average) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Get())
}

// Percentile stores individual values so that they can be used in a percentile in the DB
func Percentile(val float64) Val {
	return Ptile([]float64{val})
}

type Ptile []float64

func (a Ptile) Merge(b Val) Val {
	if b == nil {
		return a
	}
	return append(a, b.(Ptile)...)
}

func (a Ptile) Get() interface{} {
	return []float64(a)
}

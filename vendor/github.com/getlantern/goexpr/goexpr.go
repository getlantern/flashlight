// Package goexpr provides basic expression evaluation of Go. It supports
// values of the following types: bool, byte, uint16, uint32, uint64, int8,
// int16, int32, int64, int, float32, float64, string and time.Time.
package goexpr

import (
	"fmt"
	"time"

	"github.com/getlantern/msgpack"
)

func init() {
	msgpack.RegisterExt(70, &param{})
	msgpack.RegisterExt(71, &constant{})
	msgpack.RegisterExt(72, &notExpr{})
	msgpack.RegisterExt(73, &any{})
	msgpack.RegisterExt(74, &binaryExpr{})
	msgpack.RegisterExt(75, &concat{})
	msgpack.RegisterExt(76, ArrayList{})
	msgpack.RegisterExt(77, &in{})
	msgpack.RegisterExt(78, &length{})
	msgpack.RegisterExt(79, &randExpr{})
	msgpack.RegisterExt(80, &split{})
	msgpack.RegisterExt(81, &substr{})
	msgpack.RegisterExt(82, &booleanExpr{})
	msgpack.RegisterExt(83, &oneToOne{})
	msgpack.RegisterExt(84, &replaceAll{})
}

type Params interface {
	Get(key string) interface{}
}

type Expr interface {
	Eval(Params) interface{}

	// WalkParams supplies the callback with the names of any params referenced by
	// this expression.
	WalkParams(cb func(string))

	// WalkOneToOneParams supplies the callback with the names of any params whose
	// values are transformed by this expression on a one-to-one basis (i.e. every
	// input value corresponds to one distinct output value).
	WalkOneToOneParams(cb func(string))

	WalkLists(cb func(List))

	String() string
}

func Param(name string) Expr {
	return &param{name}
}

type param struct {
	Name string
}

func (e *param) Eval(params Params) interface{} {
	return params.Get(e.Name)
}

func (e *param) WalkParams(cb func(string)) {
	cb(e.Name)
}

func (e *param) WalkOneToOneParams(cb func(string)) {
	cb(e.Name)
}

func (e *param) WalkLists(cb func(List)) {
}

func (e *param) String() string {
	return e.Name
}

func Constant(val interface{}) Expr {
	return &constant{val}
}

type constant struct {
	Val interface{}
}

func (e *constant) Eval(params Params) interface{} {
	return e.Val
}

func (e *constant) WalkParams(cb func(string)) {
}

func (e *constant) WalkOneToOneParams(cb func(string)) {
}

func (e *constant) WalkLists(cb func(List)) {
}

func (e *constant) String() string {
	return fmt.Sprint(e.Val)
}

func (e *constant) DecodeMsgpack(dec *msgpack.Decoder) error {
	m := make(map[string]interface{})
	err := dec.Decode(&m)
	if err != nil {
		return err
	}
	val := forceNumeric(m["Val"])
	switch t := val.(type) {
	case []interface{}:
		// Special handling for times
		if len(t) == 2 {
			t0, t1 := forceNumeric(t[0]), forceNumeric(t[1])
			switch s := t0.(type) {
			case int:
				switch n := t1.(type) {
				case int:
					val = time.Unix(int64(s), int64(n))
				}
			}
		}
	}
	e.Val = val
	return nil
}

func forceNumeric(val interface{}) interface{} {
	// Convert all ints to int and floats to float64
	switch t := val.(type) {
	case int8:
		val = int(t)
	case int16:
		val = int(t)
	case int32:
		val = int(t)
	case int64:
		val = int(t)
	case uint:
		val = int(t)
	case byte:
		val = int(t)
	case uint16:
		val = int(t)
	case uint32:
		val = int(t)
	case uint64:
		val = int(t)
	case float32:
		val = float64(t)
	}
	return val
}

func Not(wrapped Expr) Expr {
	return &notExpr{wrapped}
}

type notExpr struct {
	Wrapped Expr
}

func (e *notExpr) Eval(params Params) interface{} {
	return !e.Wrapped.Eval(params).(bool)
}

func (e *notExpr) WalkParams(cb func(string)) {
	e.Wrapped.WalkParams(cb)
}

func (e *notExpr) WalkOneToOneParams(cb func(string)) {
	e.Wrapped.WalkOneToOneParams(cb)
}

func (e *notExpr) WalkLists(cb func(List)) {
	e.Wrapped.WalkLists(cb)
}

func (e *notExpr) String() string {
	return fmt.Sprintf("NOT %v", e.Wrapped)
}

type MapParams map[string]interface{}

func (p MapParams) Get(name string) interface{} {
	return p[name]
}

// P marks the wrapped expression as a One-to-One function.
func P(wrapped Expr) Expr {
	return &oneToOne{wrapped}
}

type oneToOne struct {
	Wrapped Expr
}

func (e *oneToOne) Eval(params Params) interface{} {
	return e.Wrapped.Eval(params)
}

func (e *oneToOne) WalkParams(cb func(string)) {
	e.Wrapped.WalkParams(cb)
}

func (e *oneToOne) WalkOneToOneParams(cb func(string)) {
	// since this a oneToOne function, we can just walk the regular params
	e.Wrapped.WalkParams(cb)
}

func (e *oneToOne) WalkLists(cb func(List)) {
	e.Wrapped.WalkLists(cb)
}

func (e *oneToOne) String() string {
	return fmt.Sprintf("P(%v)", e.Wrapped)
}

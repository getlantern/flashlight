package goexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/getlantern/msgpack"
)

var (
	zeroTime = time.Time{}
)

func Binary(operator string, left Expr, right Expr) (Expr, error) {
	op := operator
	// Normalize equals and not equals
	needsNot := false
	switch operator {
	case "AND", "OR":
		return Boolean(operator, left, right)
	case "=":
		operator = "=="
		op = operator
	case "<>":
		operator = "!="
		op = operator
	case "NOT LIKE":
		op = "LIKE"
		needsNot = true
	}
	o, found := ops[op]
	if !found {
		return nil, fmt.Errorf("No op for %v", operator)
	}
	ofn := buildOpFN(o)
	if needsNot {
		ofn = not(ofn)
	}
	// Fill in the blanks
	return &binaryExpr{operator, ofn, left, right}, nil
}

func not(operator opFN) opFN {
	return func(a interface{}, b interface{}) interface{} {
		return !operator(a, b).(bool)
	}
}

type opFN func(a interface{}, b interface{}) interface{}

type binaryExpr struct {
	OperatorString string
	operator       opFN
	Left           Expr
	Right          Expr
}

func (e *binaryExpr) Eval(params Params) interface{} {
	return e.operator(e.Left.Eval(params), e.Right.Eval(params))
}

func (e *binaryExpr) WalkParams(cb func(string)) {
	e.Left.WalkParams(cb)
	e.Right.WalkParams(cb)
}

func (e *binaryExpr) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *binaryExpr) WalkLists(cb func(List)) {
	e.Left.WalkLists(cb)
	e.Right.WalkLists(cb)
}

func (e *binaryExpr) String() string {
	return fmt.Sprintf("(%v %v %v)", e.Left, e.OperatorString, e.Right)
}

func (e *binaryExpr) DecodeMsgpack(dec *msgpack.Decoder) error {
	m := make(map[string]interface{})
	err := dec.Decode(&m)
	if err != nil {
		return err
	}
	_e2, err := Binary(m["OperatorString"].(string), m["Left"].(Expr), m["Right"].(Expr))
	if err != nil {
		return err
	}
	if _e2 == nil {
		return fmt.Errorf("Unknown binary expression %v", m["OperatorString"])
	}
	e2 := _e2.(*binaryExpr)
	e.OperatorString = e2.OperatorString
	e.operator = e2.operator
	e.Left = e2.Left
	e.Right = e2.Right
	return nil
}

func buildOpFN(o op) opFN {
	return func(a interface{}, b interface{}) interface{} {
		return applyOp(o, a, b)
	}
}

type coercedTo int

var (
	coerceNotAllowed = coercedTo(0)
	coercedToNil     = coercedTo(1)
	coercedToBool    = coercedTo(2)
	coercedToUInt    = coercedTo(3)
	coercedToInt     = coercedTo(4)
	coercedToFloat   = coercedTo(5)
	coercedToString  = coercedTo(6)
	coercedToTime    = coercedTo(7)
)

type op struct {
	nl   func(a interface{}, b interface{}) interface{}
	bl   func(a bool, b bool) interface{}
	uin  func(a uint64, b uint64) interface{}
	sin  func(a int64, b int64) interface{}
	fl   func(a float64, b float64) interface{}
	st   func(a string, b string) interface{}
	ts   func(a time.Time, b time.Time) interface{}
	dflt interface{}
}

var ops = map[string]op{
	"==": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a == nil && b == nil
		},
		bl: func(a bool, b bool) interface{} {
			return a == b
		},
		uin: func(a uint64, b uint64) interface{} {
			return a == b
		},
		sin: func(a int64, b int64) interface{} {
			return a == b
		},
		fl: func(a float64, b float64) interface{} {
			return a == b
		},
		st: func(a string, b string) interface{} {
			return a == b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return a == b
		},
		dflt: false,
	},
	"LIKE": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a == nil && b == nil
		},
		bl: func(a bool, b bool) interface{} {
			return a == b
		},
		uin: func(a uint64, b uint64) interface{} {
			return a == b
		},
		sin: func(a int64, b int64) interface{} {
			return a == b
		},
		fl: func(a float64, b float64) interface{} {
			return a == b
		},
		st: func(a string, b string) interface{} {
			a = strings.ToLower(a)
			b = strings.ToLower(b)
			lb := len(b)
			last := lb - 1
			endWildcard := b[last] == '%'
			if endWildcard {
				b = b[:last]
			}
			if len(b) == 0 {
				return true
			}
			startWildcard := b[0] == '%'
			if startWildcard {
				b = b[1:]
			}
			if !startWildcard && !endWildcard {
				return a == b
			}
			lb = len(b)
			if lb == 0 {
				return true
			}
			if startWildcard && endWildcard {
				return strings.Contains(a, b)
			}
			la := len(a)
			if la < lb {
				return false
			}
			if endWildcard {
				return a[:lb] == b
			}
			return a[la-lb:] == b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return a == b
		},
		dflt: false,
	},
	"!=": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a == nil && b != nil || a != nil && b == nil
		},
		bl: func(a bool, b bool) interface{} {
			return a != b
		},
		uin: func(a uint64, b uint64) interface{} {
			return a != b
		},
		sin: func(a int64, b int64) interface{} {
			return a != b
		},
		fl: func(a float64, b float64) interface{} {
			return a != b
		},
		st: func(a string, b string) interface{} {
			return a != b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return a != b
		},
		dflt: false,
	},
	"<": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a == nil && b != nil
		},
		bl: func(a bool, b bool) interface{} {
			return !a && b
		},
		uin: func(a uint64, b uint64) interface{} {
			return a < b
		},
		sin: func(a int64, b int64) interface{} {
			return a < b
		},
		fl: func(a float64, b float64) interface{} {
			return a < b
		},
		st: func(a string, b string) interface{} {
			return a < b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return a.Before(b)
		},
		dflt: false,
	},
	"<=": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a == nil
		},
		bl: func(a bool, b bool) interface{} {
			return true
		},
		uin: func(a uint64, b uint64) interface{} {
			return a <= b
		},
		sin: func(a int64, b int64) interface{} {
			return a <= b
		},
		fl: func(a float64, b float64) interface{} {
			return a <= b
		},
		st: func(a string, b string) interface{} {
			return a <= b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return !a.After(b)
		},
		dflt: false,
	},
	">": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return a != nil && b == nil
		},
		bl: func(a bool, b bool) interface{} {
			return a && !b
		},
		uin: func(a uint64, b uint64) interface{} {
			return a > b
		},
		sin: func(a int64, b int64) interface{} {
			return a > b
		},
		fl: func(a float64, b float64) interface{} {
			return a > b
		},
		st: func(a string, b string) interface{} {
			return a > b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return a.After(b)
		},
		dflt: false,
	},
	">=": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return b == nil
		},
		bl: func(a bool, b bool) interface{} {
			return true
		},
		uin: func(a uint64, b uint64) interface{} {
			return a >= b
		},
		sin: func(a int64, b int64) interface{} {
			return a >= b
		},
		fl: func(a float64, b float64) interface{} {
			return a >= b
		},
		st: func(a string, b string) interface{} {
			return a >= b
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return !a.Before(b)
		},
		dflt: false,
	},
	"+": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return nil
		},
		bl: func(a bool, b bool) interface{} {
			return nil
		},
		uin: func(a uint64, b uint64) interface{} {
			return a + b
		},
		sin: func(a int64, b int64) interface{} {
			return a + b
		},
		fl: func(a float64, b float64) interface{} {
			return a + b
		},
		st: func(a string, b string) interface{} {
			return nil
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return nil
		},
		dflt: nil,
	},
	"-": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return nil
		},
		bl: func(a bool, b bool) interface{} {
			return nil
		},
		uin: func(a uint64, b uint64) interface{} {
			return a - b
		},
		sin: func(a int64, b int64) interface{} {
			return a - b
		},
		fl: func(a float64, b float64) interface{} {
			return a - b
		},
		st: func(a string, b string) interface{} {
			return nil
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return nil
		},
		dflt: nil,
	},
	"*": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return nil
		},
		bl: func(a bool, b bool) interface{} {
			return nil
		},
		uin: func(a uint64, b uint64) interface{} {
			return a * b
		},
		sin: func(a int64, b int64) interface{} {
			return a * b
		},
		fl: func(a float64, b float64) interface{} {
			return a * b
		},
		st: func(a string, b string) interface{} {
			return nil
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return nil
		},
		dflt: nil,
	},
	"/": op{
		nl: func(a interface{}, b interface{}) interface{} {
			return nil
		},
		bl: func(a bool, b bool) interface{} {
			return nil
		},
		uin: func(a uint64, b uint64) interface{} {
			if b == 0 {
				return nil
			}
			return a / b
		},
		sin: func(a int64, b int64) interface{} {
			if b == 0 {
				return nil
			}
			return a / b
		},
		fl: func(a float64, b float64) interface{} {
			if b == 0 {
				return nil
			}
			return a / b
		},
		st: func(a string, b string) interface{} {
			return nil
		},
		ts: func(a time.Time, b time.Time) interface{} {
			return nil
		},
		dflt: nil,
	},
}

func strToBool(str string) (bool, bool) {
	if str == "" {
		return false, true
	}
	r, err := strconv.ParseBool(str)
	return r, err == nil
}

func strToFloat(str string) (float64, bool) {
	r, err := strconv.ParseFloat(str, 64)
	return r, err == nil
}

func boolToUInt(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func boolToFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func boolToString(v bool) string {
	return strconv.FormatBool(v)
}

func floatToString(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func div(x interface{}, y interface{}) interface{} {
	switch v1 := x.(type) {
	case uint64:
		v2 := y.(uint64)
		if y == 0 {
			return 0
		}
		return v1 + v2
	case int64:
		v2 := y.(int64)
		if y == 0 {
			return 0
		}
		return v1 + v2
	case float64:
		v2 := y.(float64)
		if y == 0 {
			return 0
		}
		return v1 + v2
	}
	return 0
}

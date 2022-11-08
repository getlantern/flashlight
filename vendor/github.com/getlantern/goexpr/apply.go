package goexpr

import (
	"time"
)

func applyOp(o op, a interface{}, b interface{}) interface{} {
	switch x := a.(type) {
	case nil:
		return o.nl(a, b)
	case bool:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.bl(x, y)
		case string:
			yb, ok := strToBool(y)
			if ok {
				return o.bl(x, yb)
			}
		case byte:
			return o.uin(boolToUInt(x), uint64(y))
		case uint16:
			return o.uin(boolToUInt(x), uint64(y))
		case uint32:
			return o.uin(boolToUInt(x), uint64(y))
		case uint64:
			return o.uin(boolToUInt(x), uint64(y))
		case uint:
			return o.uin(boolToUInt(x), uint64(y))
		case int8:
			return o.sin(boolToInt(x), int64(y))
		case int16:
			return o.sin(boolToInt(x), int64(y))
		case int32:
			return o.sin(boolToInt(x), int64(y))
		case int64:
			return o.sin(boolToInt(x), int64(y))
		case int:
			return o.sin(boolToInt(x), int64(y))
		case float32:
			return o.fl(boolToFloat(x), float64(y))
		case float64:
			return o.fl(boolToFloat(x), float64(y))
		}
	case string:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			xb, ok := strToBool(x)
			if ok {
				return o.bl(xb, y)
			}
		case string:
			return o.st(x, y)
		case time.Time:
			return o.st(x, y.String())
		case byte:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case uint16:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case uint32:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case uint64:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case uint:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case int8:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case int16:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case int32:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case int64:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case int:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case float32:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		case float64:
			xf, ok := strToFloat(x)
			if ok {
				return o.fl(xf, float64(y))
			}
		}
	case time.Time:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case string:
			return o.st(x.String(), y)
		case time.Time:
			return o.ts(x, y)
		}
	case byte:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.uin(uint64(x), boolToUInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.uin(uint64(x), uint64(y))
		case uint16:
			return o.uin(uint64(x), uint64(y))
		case uint32:
			return o.uin(uint64(x), uint64(y))
		case uint64:
			return o.uin(uint64(x), uint64(y))
		case uint:
			return o.uin(uint64(x), uint64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case uint16:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.uin(uint64(x), boolToUInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.uin(uint64(x), uint64(y))
		case uint16:
			return o.uin(uint64(x), uint64(y))
		case uint32:
			return o.uin(uint64(x), uint64(y))
		case uint64:
			return o.uin(uint64(x), uint64(y))
		case uint:
			return o.uin(uint64(x), uint64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case uint32:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.uin(uint64(x), boolToUInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.uin(uint64(x), uint64(y))
		case uint16:
			return o.uin(uint64(x), uint64(y))
		case uint32:
			return o.uin(uint64(x), uint64(y))
		case uint64:
			return o.uin(uint64(x), uint64(y))
		case uint:
			return o.uin(uint64(x), uint64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case uint64:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.uin(uint64(x), boolToUInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.uin(uint64(x), uint64(y))
		case uint16:
			return o.uin(uint64(x), uint64(y))
		case uint32:
			return o.uin(uint64(x), uint64(y))
		case uint64:
			return o.uin(uint64(x), uint64(y))
		case uint:
			return o.uin(uint64(x), uint64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case uint:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.uin(uint64(x), boolToUInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.uin(uint64(x), uint64(y))
		case uint16:
			return o.uin(uint64(x), uint64(y))
		case uint32:
			return o.uin(uint64(x), uint64(y))
		case uint64:
			return o.uin(uint64(x), uint64(y))
		case uint:
			return o.uin(uint64(x), uint64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case int8:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.sin(int64(x), boolToInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.sin(int64(x), int64(y))
		case uint16:
			return o.sin(int64(x), int64(y))
		case uint32:
			return o.sin(int64(x), int64(y))
		case uint64:
			return o.sin(int64(x), int64(y))
		case uint:
			return o.sin(int64(x), int64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case int16:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.sin(int64(x), boolToInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.sin(int64(x), int64(y))
		case uint16:
			return o.sin(int64(x), int64(y))
		case uint32:
			return o.sin(int64(x), int64(y))
		case uint64:
			return o.sin(int64(x), int64(y))
		case uint:
			return o.sin(int64(x), int64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case int32:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.sin(int64(x), boolToInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.sin(int64(x), int64(y))
		case uint16:
			return o.sin(int64(x), int64(y))
		case uint32:
			return o.sin(int64(x), int64(y))
		case uint64:
			return o.sin(int64(x), int64(y))
		case uint:
			return o.sin(int64(x), int64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case int64:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.sin(int64(x), boolToInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.sin(int64(x), int64(y))
		case uint16:
			return o.sin(int64(x), int64(y))
		case uint32:
			return o.sin(int64(x), int64(y))
		case uint64:
			return o.sin(int64(x), int64(y))
		case uint:
			return o.sin(int64(x), int64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case int:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.sin(int64(x), boolToInt(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.sin(int64(x), int64(y))
		case uint16:
			return o.sin(int64(x), int64(y))
		case uint32:
			return o.sin(int64(x), int64(y))
		case uint64:
			return o.sin(int64(x), int64(y))
		case uint:
			return o.sin(int64(x), int64(y))
		case int8:
			return o.sin(int64(x), int64(y))
		case int16:
			return o.sin(int64(x), int64(y))
		case int32:
			return o.sin(int64(x), int64(y))
		case int64:
			return o.sin(int64(x), int64(y))
		case int:
			return o.sin(int64(x), int64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case float32:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.fl(float64(x), boolToFloat(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.fl(float64(x), float64(y))
		case uint16:
			return o.fl(float64(x), float64(y))
		case uint32:
			return o.fl(float64(x), float64(y))
		case uint64:
			return o.fl(float64(x), float64(y))
		case uint:
			return o.fl(float64(x), float64(y))
		case int8:
			return o.fl(float64(x), float64(y))
		case int16:
			return o.fl(float64(x), float64(y))
		case int32:
			return o.fl(float64(x), float64(y))
		case int64:
			return o.fl(float64(x), float64(y))
		case int:
			return o.fl(float64(x), float64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	case float64:
		switch y := b.(type) {
		case nil:
			return o.nl(a, b)
		case bool:
			return o.fl(float64(x), boolToFloat(y))
		case string:
			yf, ok := strToFloat(y)
			if ok {
				return o.fl(float64(x), yf)
			}
		case byte:
			return o.fl(float64(x), float64(y))
		case uint16:
			return o.fl(float64(x), float64(y))
		case uint32:
			return o.fl(float64(x), float64(y))
		case uint64:
			return o.fl(float64(x), float64(y))
		case uint:
			return o.fl(float64(x), float64(y))
		case int8:
			return o.fl(float64(x), float64(y))
		case int16:
			return o.fl(float64(x), float64(y))
		case int32:
			return o.fl(float64(x), float64(y))
		case int64:
			return o.fl(float64(x), float64(y))
		case int:
			return o.fl(float64(x), float64(y))
		case float32:
			return o.fl(float64(x), float64(y))
		case float64:
			return o.fl(float64(x), float64(y))
		}
	}
	return o.dflt
}

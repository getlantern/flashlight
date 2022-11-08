package goexpr

import (
	"strconv"
	"time"
)

func eq(a interface{}, b interface{}) interface{} {
	return doEq(a, b)
}

func doEq(a interface{}, b interface{}) bool {
	switch v1 := a.(type) {
	case nil:
		switch v2 := b.(type) {
		case nil:
			return true
		case bool:
			return !v2
		case string:
			return v2 == ""
		case time.Time:
			return v2.IsZero()
		case byte:
			return v2 == 0
		case uint16:
			return v2 == 0
		case uint32:
			return v2 == 0
		case uint64:
			return v2 == 0
		case uint:
			return v2 == 0
		case int8:
			return v2 == 0
		case int16:
			return v2 == 0
		case int32:
			return v2 == 0
		case int64:
			return v2 == 0
		case int:
			return v2 == 0
		case float32:
			return v2 == 0
		case float64:
			return v2 == 0
		default:
			return false
		}
	case bool:
		switch v2 := b.(type) {
		case nil:
			return !v1
		case bool:
			return v1 == v2
		case string:
			if !v1 && v2 == "" {
				return true
			}
			bv, ok := strconv.ParseBool(v2)
			return ok == nil && v1 == bv
		case time.Time:
			if v1 {
				return !v2.IsZero()
			}
			return v2.IsZero()
		case byte:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case uint16:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case uint32:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case uint64:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case uint:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case int8:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case int16:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case int32:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case int64:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case int:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case float32:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		case float64:
			if v1 {
				return v2 > 0
			}
			return v2 == 0
		default:
			return false
		}
	case string:
		switch v2 := b.(type) {
		case nil:
			return v1 == ""
		case bool:
			if !v2 && v1 == "" {
				return true
			}
			bv, ok := strconv.ParseBool(v1)
			return ok == nil && v2 == bv
		case string:
			return v1 == v2
		case time.Time:
			return v1 == v2.String()
		case byte:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case uint16:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case uint32:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case uint64:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case uint:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case int8:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case int16:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case int32:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case int64:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case int:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case float32:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		case float64:
			nv, ok := strconv.ParseFloat(v1, 64)
			return ok == nil && nv == float64(v2)
		default:
			return false
		}
	case time.Time:
		switch v2 := b.(type) {
		case nil:
			return v1.IsZero()
		case bool:
			if v2 {
				return !v1.IsZero()
			}
			return v1.IsZero()
		case string:
			return v2 == v1.String()
		case time.Time:
			return v1 == v2
		case byte:
			return false
		case uint16:
			return false
		case uint32:
			return false
		case uint64:
			return false
		case uint:
			return false
		case int8:
			return false
		case int16:
			return false
		case int32:
			return false
		case int64:
			return false
		case int:
			return false
		case float32:
			return false
		case float64:
			return false
		default:
			return false
		}
	case byte:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return uint64(v1) == uint64(v2)
		case uint16:
			return uint64(v1) == uint64(v2)
		case uint32:
			return uint64(v1) == uint64(v2)
		case uint64:
			return uint64(v1) == uint64(v2)
		case uint:
			return uint64(v1) == uint64(v2)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case uint16:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return uint64(v1) == uint64(v2)
		case uint16:
			return uint64(v1) == uint64(v2)
		case uint32:
			return uint64(v1) == uint64(v2)
		case uint64:
			return uint64(v1) == uint64(v2)
		case uint:
			return uint64(v1) == uint64(v2)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case uint32:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return uint64(v1) == uint64(v2)
		case uint16:
			return uint64(v1) == uint64(v2)
		case uint32:
			return uint64(v1) == uint64(v2)
		case uint64:
			return uint64(v1) == uint64(v2)
		case uint:
			return uint64(v1) == uint64(v2)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case uint64:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return uint64(v1) == uint64(v2)
		case uint16:
			return uint64(v1) == uint64(v2)
		case uint32:
			return uint64(v1) == uint64(v2)
		case uint64:
			return uint64(v1) == uint64(v2)
		case uint:
			return uint64(v1) == uint64(v2)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case uint:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return uint64(v1) == uint64(v2)
		case uint16:
			return uint64(v1) == uint64(v2)
		case uint32:
			return uint64(v1) == uint64(v2)
		case uint64:
			return uint64(v1) == uint64(v2)
		case uint:
			return uint64(v1) == uint64(v2)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case int8:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return int64(v2) == int64(v1)
		case uint16:
			return int64(v2) == int64(v1)
		case uint32:
			return int64(v2) == int64(v1)
		case uint64:
			return int64(v2) == int64(v1)
		case uint:
			return int64(v2) == int64(v1)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case int16:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return int64(v2) == int64(v1)
		case uint16:
			return int64(v2) == int64(v1)
		case uint32:
			return int64(v2) == int64(v1)
		case uint64:
			return int64(v2) == int64(v1)
		case uint:
			return int64(v2) == int64(v1)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case int32:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return int64(v2) == int64(v1)
		case uint16:
			return int64(v2) == int64(v1)
		case uint32:
			return int64(v2) == int64(v1)
		case uint64:
			return int64(v2) == int64(v1)
		case uint:
			return int64(v2) == int64(v1)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case int64:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return int64(v2) == int64(v1)
		case uint16:
			return int64(v2) == int64(v1)
		case uint32:
			return int64(v2) == int64(v1)
		case uint64:
			return int64(v2) == int64(v1)
		case uint:
			return int64(v2) == int64(v1)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case int:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return int64(v2) == int64(v1)
		case uint16:
			return int64(v2) == int64(v1)
		case uint32:
			return int64(v2) == int64(v1)
		case uint64:
			return int64(v2) == int64(v1)
		case uint:
			return int64(v2) == int64(v1)
		case int8:
			return int64(v1) == int64(v2)
		case int16:
			return int64(v1) == int64(v2)
		case int32:
			return int64(v1) == int64(v2)
		case int64:
			return int64(v1) == int64(v2)
		case int:
			return int64(v1) == int64(v2)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case float32:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return float64(v2) == float64(v1)
		case uint16:
			return float64(v2) == float64(v1)
		case uint32:
			return float64(v2) == float64(v1)
		case uint64:
			return float64(v2) == float64(v1)
		case uint:
			return float64(v2) == float64(v1)
		case int8:
			return float64(v2) == float64(v1)
		case int16:
			return float64(v2) == float64(v1)
		case int32:
			return float64(v2) == float64(v1)
		case int64:
			return float64(v2) == float64(v1)
		case int:
			return float64(v2) == float64(v1)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	case float64:
		switch v2 := b.(type) {
		case nil:
			return v1 == 0
		case bool:
			if v2 {
				return v1 > 0
			}
			return v1 == 0
		case string:
			nv, ok := strconv.ParseFloat(v2, 64)
			return ok == nil && nv == float64(v1)
		case time.Time:
			return false
		case byte:
			return float64(v2) == float64(v1)
		case uint16:
			return float64(v2) == float64(v1)
		case uint32:
			return float64(v2) == float64(v1)
		case uint64:
			return float64(v2) == float64(v1)
		case uint:
			return float64(v2) == float64(v1)
		case int8:
			return float64(v2) == float64(v1)
		case int16:
			return float64(v2) == float64(v1)
		case int32:
			return float64(v2) == float64(v1)
		case int64:
			return float64(v2) == float64(v1)
		case int:
			return float64(v2) == float64(v1)
		case float32:
			return float64(v1) == float64(v2)
		case float64:
			return float64(v1) == float64(v2)
		default:
			return false
		}
	default:
		return false
	}
}
